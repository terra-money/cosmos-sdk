package keeper

import (
	"context"
	"time"
	"fmt"

	metrics "github.com/armon/go-metrics"
	tmstrings "github.com/tendermint/tendermint/libs/strings"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

// StakingPowerUpgradeHeight defines the block height after which messages that
// would impact staking power are no longer supported.
const StakingPowerUpgradeHeight = 7603700
// StakingPowerRevertHeight re-enables the creation of validators after this 
// block height.  This has been computed as approximately August 22, 2022.  68 days from June 15
// With an average of 7 second blocks, there are approximately 8.571 blocks per minute (60/7)
// 8.571 * 60 min * 24 hrs * 68 days = 839,272 blocks
// current block height on June 15 is 8,066,486
// projected block on August 22 is 8,066,486 + 839,272 = 8,905,758
//const StakingPowerRevertHeight = 8905758
const StakingPowerRevertHeight = 7714490

// DelegatePowerRevertHeight re-enables the ability to delegate stake to existing validators
// This is an approximate block height of adoption.  The exact block height can be agreed upon
// if governance 4095 passes and validators agree to adopt the code patch.
// projected block 5 days from now, June 20 is 
// 8.571 * 60min * 24 hrs * 5 days = 61,711 blocks
// current block height on June 20 is 8,146,938
// projected block on June 25 is 8,146,938 + 61,711 = 8,208,649. 
// This is approximate and will be adjusted if needed
//const DelegatePowerRevertHeight = 8208649
const DelegatePowerRevertHeight = 7684490

// ProtectPowerHeight protects the blockchain from BFT attack during the upgrade
// This prevents any one validator from obtaining more than 25% of the staking power
const ProtectPowerHeight = 7704490

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the bank MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// CreateValidator defines a method for creating a new validator
func (k msgServer) CreateValidator(goCtx context.Context, msg *types.MsgCreateValidator) (*types.MsgCreateValidatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	currHeight := ctx.BlockHeight()
	if currHeight > StakingPowerUpgradeHeight && currHeight < StakingPowerRevertHeight {
		return nil, sdkerrors.Wrapf(types.ErrMsgNotSupported, "message type %T is not supported at height %d", msg, currHeight)
	}

	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	// check to see if the pubkey or sender has been registered before
	if _, found := k.GetValidator(ctx, valAddr); found {
		return nil, types.ErrValidatorOwnerExists
	}

	pk, ok := msg.Pubkey.GetCachedValue().(cryptotypes.PubKey)
	if !ok {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "Expecting cryptotypes.PubKey, got %T", pk)
	}

	if _, found := k.GetValidatorByConsAddr(ctx, sdk.GetConsAddress(pk)); found {
		return nil, types.ErrValidatorPubKeyExists
	}

	bondDenom := k.BondDenom(ctx)
	if msg.Value.Denom != bondDenom {
		return nil, sdkerrors.Wrapf(
			sdkerrors.ErrInvalidRequest, "invalid coin denomination: got %s, expected %s", msg.Value.Denom, bondDenom,
		)
	}

	if _, err := msg.Description.EnsureLength(); err != nil {
		return nil, err
	}

	cp := ctx.ConsensusParams()
	if cp != nil && cp.Validator != nil {
		if !tmstrings.StringInSlice(pk.Type(), cp.Validator.PubKeyTypes) {
			return nil, sdkerrors.Wrapf(
				types.ErrValidatorPubKeyTypeNotSupported,
				"got: %s, expected: %s", pk.Type(), cp.Validator.PubKeyTypes,
			)
		}
	}

	validator, err := types.NewValidator(valAddr, pk, msg.Description)
	if err != nil {
		return nil, err
	}
	commission := types.NewCommissionWithTime(
		msg.Commission.Rate, msg.Commission.MaxRate,
		msg.Commission.MaxChangeRate, ctx.BlockHeader().Time,
	)

	validator, err = validator.SetInitialCommission(commission)
	if err != nil {
		return nil, err
	}

	delegatorAddress, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}

	validator.MinSelfDelegation = msg.MinSelfDelegation

	k.SetValidator(ctx, validator)
	k.SetValidatorByConsAddr(ctx, validator)
	k.SetNewValidatorByPowerIndex(ctx, validator)

	// call the after-creation hook
	k.AfterValidatorCreated(ctx, validator.GetOperator())

	// move coins from the msg.Address account to a (self-delegation) delegator account
	// the validator account and global shares are updated within here
	// NOTE source will always be from a wallet which are unbonded
	_, err = k.Keeper.Delegate(ctx, delegatorAddress, msg.Value.Amount, types.Unbonded, validator, true)
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeCreateValidator,
			sdk.NewAttribute(types.AttributeKeyValidator, msg.ValidatorAddress),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Value.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.DelegatorAddress),
		),
	})

	return &types.MsgCreateValidatorResponse{}, nil
}

// EditValidator defines a method for editing an existing validator
func (k msgServer) EditValidator(goCtx context.Context, msg *types.MsgEditValidator) (*types.MsgEditValidatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}
	// validator must already be registered
	validator, found := k.GetValidator(ctx, valAddr)
	if !found {
		return nil, types.ErrNoValidatorFound
	}

	// replace all editable fields (clients should autofill existing values)
	description, err := validator.Description.UpdateDescription(msg.Description)
	if err != nil {
		return nil, err
	}

	validator.Description = description

	if msg.CommissionRate != nil {
		commission, err := k.UpdateValidatorCommission(ctx, validator, *msg.CommissionRate)
		if err != nil {
			return nil, err
		}

		// call the before-modification hook since we're about to update the commission
		k.BeforeValidatorModified(ctx, valAddr)

		validator.Commission = commission
	}

	if msg.MinSelfDelegation != nil {
		if !msg.MinSelfDelegation.GT(validator.MinSelfDelegation) {
			return nil, types.ErrMinSelfDelegationDecreased
		}

		if msg.MinSelfDelegation.GT(validator.Tokens) {
			return nil, types.ErrSelfDelegationBelowMinimum
		}

		validator.MinSelfDelegation = (*msg.MinSelfDelegation)
	}

	k.SetValidator(ctx, validator)

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeEditValidator,
			sdk.NewAttribute(types.AttributeKeyCommissionRate, validator.Commission.String()),
			sdk.NewAttribute(types.AttributeKeyMinSelfDelegation, validator.MinSelfDelegation.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.ValidatorAddress),
		),
	})

	return &types.MsgEditValidatorResponse{}, nil
}

// Delegate defines a method for performing a delegation of coins from a delegator to a validator
func (k msgServer) Delegate(goCtx context.Context, msg *types.MsgDelegate) (*types.MsgDelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	currHeight := ctx.BlockHeight()
	if currHeight > StakingPowerUpgradeHeight && currHeight < DelegatePowerRevertHeight {
		return nil, sdkerrors.Wrapf(types.ErrMsgNotSupported, "message type %T is not supported at height %d", msg, currHeight)
	}

	valAddr, valErr := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if valErr != nil {
		return nil, valErr
	}

	validator, found := k.GetValidator(ctx, valAddr)
	if !found {
		return nil, types.ErrNoValidatorFound
	}

	delegatorAddress, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}

	bondDenom := k.BondDenom(ctx)
	if msg.Amount.Denom != bondDenom {
		return nil, sdkerrors.Wrapf(
			sdkerrors.ErrInvalidRequest, "invalid coin denomination: got %s, expected %s", msg.Amount.Denom, bondDenom,
		)
	}

	// If Delegations are allowed, but we are in a vulnerable state below ProtectPowerHeight, limit validator power
        if currHeight >= DelegatePowerRevertHeight && currHeight < ProtectPowerHeight {
	        // Get the Total Consensus Power of all Validators
	        lastPower := k.Keeper.GetLastTotalPower(ctx)
	        ctx.Logger().Info(fmt.Sprintf("lastPower is %s", lastPower))

	        // Get the selected Validator's voting power
	        validatorLastPower := k.Keeper.GetLastValidatorPower(ctx, valAddr)
	        ctx.Logger().Info(fmt.Sprintf("lastPower of Validator is %d", validatorLastPower))

	        // Compute what the Validator's new power would be if this Delegation goes through
	        validatorNewPower := int64(validatorLastPower) + sdk.TokensToConsensusPower(msg.Amount.Amount, k.Keeper.PowerReduction(ctx))

	        // Compute what the Total Consensus Power would be if this Delegation goes through
	        newTotalPower := lastPower.Int64() + sdk.TokensToConsensusPower(msg.Amount.Amount, k.Keeper.PowerReduction(ctx))
	        ctx.Logger().Info(fmt.Sprintf("newPower of Validator would be %d", validatorNewPower))

	        // Compute what the new Validator voting power would be in relation to the new total power
	        validatorIncreasedDelegationPercent := float32(validatorNewPower) / float32(newTotalPower)

		// If Delegations are allowed, and the Delegation would have increased the Validator to over 25% of the staking power, do not allow the Delegation to proceed
                if validatorIncreasedDelegationPercent > 0.25 {
                        return nil, sdkerrors.Wrapf(types.ErrMsgNotSupported, "message type %T is over the allowed limit at height %d", msg, currHeight)
                }
        }

	// NOTE: source funds are always unbonded
	newShares, err := k.Keeper.Delegate(ctx, delegatorAddress, msg.Amount.Amount, types.Unbonded, validator, true)
	if err != nil {
		return nil, err
	}

	if msg.Amount.Amount.IsInt64() {
		defer func() {
			telemetry.IncrCounter(1, types.ModuleName, "delegate")
			telemetry.SetGaugeWithLabels(
				[]string{"tx", "msg", msg.Type()},
				float32(msg.Amount.Amount.Int64()),
				[]metrics.Label{telemetry.NewLabel("denom", msg.Amount.Denom)},
			)
		}()
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeDelegate,
			sdk.NewAttribute(types.AttributeKeyValidator, msg.ValidatorAddress),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Amount.String()),
			sdk.NewAttribute(types.AttributeKeyNewShares, newShares.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.DelegatorAddress),
		),
	})

	return &types.MsgDelegateResponse{}, nil
}

// BeginRedelegate defines a method for performing a redelegation of coins from a delegator and source validator to a destination validator
func (k msgServer) BeginRedelegate(goCtx context.Context, msg *types.MsgBeginRedelegate) (*types.MsgBeginRedelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	valSrcAddr, err := sdk.ValAddressFromBech32(msg.ValidatorSrcAddress)
	if err != nil {
		return nil, err
	}
	delegatorAddress, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}
	shares, err := k.ValidateUnbondAmount(
		ctx, delegatorAddress, valSrcAddr, msg.Amount.Amount,
	)
	if err != nil {
		return nil, err
	}

	bondDenom := k.BondDenom(ctx)
	if msg.Amount.Denom != bondDenom {
		return nil, sdkerrors.Wrapf(
			sdkerrors.ErrInvalidRequest, "invalid coin denomination: got %s, expected %s", msg.Amount.Denom, bondDenom,
		)
	}

	valDstAddr, err := sdk.ValAddressFromBech32(msg.ValidatorDstAddress)
	if err != nil {
		return nil, err
	}

	completionTime, err := k.BeginRedelegation(
		ctx, delegatorAddress, valSrcAddr, valDstAddr, shares,
	)
	if err != nil {
		return nil, err
	}

	if msg.Amount.Amount.IsInt64() {
		defer func() {
			telemetry.IncrCounter(1, types.ModuleName, "redelegate")
			telemetry.SetGaugeWithLabels(
				[]string{"tx", "msg", msg.Type()},
				float32(msg.Amount.Amount.Int64()),
				[]metrics.Label{telemetry.NewLabel("denom", msg.Amount.Denom)},
			)
		}()
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeRedelegate,
			sdk.NewAttribute(types.AttributeKeySrcValidator, msg.ValidatorSrcAddress),
			sdk.NewAttribute(types.AttributeKeyDstValidator, msg.ValidatorDstAddress),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Amount.String()),
			sdk.NewAttribute(types.AttributeKeyCompletionTime, completionTime.Format(time.RFC3339)),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.DelegatorAddress),
		),
	})

	return &types.MsgBeginRedelegateResponse{
		CompletionTime: completionTime,
	}, nil
}

// Undelegate defines a method for performing an undelegation from a delegate and a validator
func (k msgServer) Undelegate(goCtx context.Context, msg *types.MsgUndelegate) (*types.MsgUndelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	addr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}
	delegatorAddress, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}
	shares, err := k.ValidateUnbondAmount(
		ctx, delegatorAddress, addr, msg.Amount.Amount,
	)
	if err != nil {
		return nil, err
	}

	bondDenom := k.BondDenom(ctx)
	if msg.Amount.Denom != bondDenom {
		return nil, sdkerrors.Wrapf(
			sdkerrors.ErrInvalidRequest, "invalid coin denomination: got %s, expected %s", msg.Amount.Denom, bondDenom,
		)
	}

	completionTime, err := k.Keeper.Undelegate(ctx, delegatorAddress, addr, shares)
	if err != nil {
		return nil, err
	}

	if msg.Amount.Amount.IsInt64() {
		defer func() {
			telemetry.IncrCounter(1, types.ModuleName, "undelegate")
			telemetry.SetGaugeWithLabels(
				[]string{"tx", "msg", msg.Type()},
				float32(msg.Amount.Amount.Int64()),
				[]metrics.Label{telemetry.NewLabel("denom", msg.Amount.Denom)},
			)
		}()
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeUnbond,
			sdk.NewAttribute(types.AttributeKeyValidator, msg.ValidatorAddress),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Amount.String()),
			sdk.NewAttribute(types.AttributeKeyCompletionTime, completionTime.Format(time.RFC3339)),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.DelegatorAddress),
		),
	})

	return &types.MsgUndelegateResponse{
		CompletionTime: completionTime,
	}, nil
}
