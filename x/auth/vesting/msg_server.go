package vesting

import (
	"context"
	"fmt"
	"math"

	"github.com/armon/go-metrics"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting/exported"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
)

type msgServer struct {
	keeper.AccountKeeper
	types.BankKeeper
	types.DistrKeeper
	types.StakingKeeper
}

// NewMsgServerImpl returns an implementation of the vesting MsgServer interface,
// wrapping the corresponding AccountKeeper and BankKeeper.
func NewMsgServerImpl(
	k keeper.AccountKeeper,
	bk types.BankKeeper,
	dk types.DistrKeeper,
	sk types.StakingKeeper,
) types.MsgServer {
	return &msgServer{AccountKeeper: k, BankKeeper: bk, DistrKeeper: dk, StakingKeeper: sk}
}

var _ types.MsgServer = msgServer{}

func (s msgServer) CreateVestingAccount(goCtx context.Context, msg *types.MsgCreateVestingAccount) (*types.MsgCreateVestingAccountResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ak := s.AccountKeeper
	bk := s.BankKeeper

	if err := bk.IsSendEnabledCoins(ctx, msg.Amount...); err != nil {
		return nil, err
	}

	from, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, err
	}
	to, err := sdk.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		return nil, err
	}

	if bk.BlockedAddr(to) {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive funds", msg.ToAddress)
	}

	if acc := ak.GetAccount(ctx, to); acc != nil {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s already exists", msg.ToAddress)
	}

	baseAccount := authtypes.NewBaseAccountWithAddress(to)
	baseAccount = ak.NewAccount(ctx, baseAccount).(*authtypes.BaseAccount)
	baseVestingAccount := types.NewBaseVestingAccount(baseAccount, msg.Amount.Sort(), msg.EndTime)

	var vestingAccount authtypes.AccountI
	if msg.Delayed {
		vestingAccount = types.NewDelayedVestingAccountRaw(baseVestingAccount)
	} else {
		vestingAccount = types.NewContinuousVestingAccountRaw(baseVestingAccount, ctx.BlockTime().Unix())
	}

	ak.SetAccount(ctx, vestingAccount)

	defer func() {
		telemetry.IncrCounter(1, "new", "account")

		for _, a := range msg.Amount {
			if a.Amount.IsInt64() {
				telemetry.SetGaugeWithLabels(
					[]string{"tx", "msg", "create_vesting_account"},
					float32(a.Amount.Int64()),
					[]metrics.Label{telemetry.NewLabel("denom", a.Denom)},
				)
			}
		}
	}()

	if err = bk.SendCoins(ctx, from, to, msg.Amount); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	)

	return &types.MsgCreateVestingAccountResponse{}, nil
}

func (s msgServer) CreatePermanentLockedAccount(goCtx context.Context, msg *types.MsgCreatePermanentLockedAccount) (*types.MsgCreatePermanentLockedAccountResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ak := s.AccountKeeper
	bk := s.BankKeeper

	if err := bk.IsSendEnabledCoins(ctx, msg.Amount...); err != nil {
		return nil, err
	}

	from, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, err
	}
	to, err := sdk.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		return nil, err
	}

	if bk.BlockedAddr(to) {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive funds", msg.ToAddress)
	}

	if acc := ak.GetAccount(ctx, to); acc != nil {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s already exists", msg.ToAddress)
	}

	baseAccount := authtypes.NewBaseAccountWithAddress(to)
	baseAccount = ak.NewAccount(ctx, baseAccount).(*authtypes.BaseAccount)
	vestingAccount := types.NewPermanentLockedAccount(baseAccount, msg.Amount)

	ak.SetAccount(ctx, vestingAccount)

	defer func() {
		telemetry.IncrCounter(1, "new", "account")

		for _, a := range msg.Amount {
			if a.Amount.IsInt64() {
				telemetry.SetGaugeWithLabels(
					[]string{"tx", "msg", "create_permanent_locked_account"},
					float32(a.Amount.Int64()),
					[]metrics.Label{telemetry.NewLabel("denom", a.Denom)},
				)
			}
		}
	}()

	if err = bk.SendCoins(ctx, from, to, msg.Amount); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	)

	return &types.MsgCreatePermanentLockedAccountResponse{}, nil
}

func (s msgServer) CreatePeriodicVestingAccount(goCtx context.Context, msg *types.MsgCreatePeriodicVestingAccount) (*types.MsgCreatePeriodicVestingAccountResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	ak := s.AccountKeeper
	bk := s.BankKeeper

	from, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, err
	}
	to, err := sdk.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		return nil, err
	}

	if acc := ak.GetAccount(ctx, to); acc != nil {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s already exists", msg.ToAddress)
	}

	var totalCoins sdk.Coins
	for _, period := range msg.VestingPeriods {
		totalCoins = totalCoins.Add(period.Amount...)
	}

	if err := bk.IsSendEnabledCoins(ctx, totalCoins...); err != nil {
		return nil, err
	}

	baseAccount := authtypes.NewBaseAccountWithAddress(to)
	baseAccount = ak.NewAccount(ctx, baseAccount).(*authtypes.BaseAccount)
	vestingAccount := types.NewPeriodicVestingAccount(baseAccount, totalCoins.Sort(), msg.StartTime, msg.VestingPeriods)

	ak.SetAccount(ctx, vestingAccount)

	defer func() {
		telemetry.IncrCounter(1, "new", "account")

		for _, a := range totalCoins {
			if a.Amount.IsInt64() {
				telemetry.SetGaugeWithLabels(
					[]string{"tx", "msg", "create_periodic_vesting_account"},
					float32(a.Amount.Int64()),
					[]metrics.Label{telemetry.NewLabel("denom", a.Denom)},
				)
			}
		}
	}()

	if err = bk.SendCoins(ctx, from, to, totalCoins); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	)
	return &types.MsgCreatePeriodicVestingAccountResponse{}, nil
}

func (s msgServer) DonateAllVestingTokens(goCtx context.Context, msg *types.MsgDonateAllVestingTokens) (*types.MsgDonateAllVestingTokensResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	ak := s.AccountKeeper
	dk := s.DistrKeeper
	sk := s.StakingKeeper

	from, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, err
	}

	acc := ak.GetAccount(ctx, from)
	if acc == nil {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s not exists", msg.FromAddress)
	}

	// get all delegations of an account and undust those that have less than 1 uluna
	delegations := sk.GetDelegatorDelegations(ctx, acc.GetAddress(), math.MaxUint16)
	for _, delegation := range delegations {
		validatorAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress)
		if err != nil {
			return nil, err
		}
		validator, found := sk.GetValidator(ctx, validatorAddr)
		if !found {
			return nil, fmt.Errorf("validator not found")
		}
		// Try to delete the dust delegation
		_, removedTokens := sk.RemoveValidatorTokensAndShares(ctx, validator, delegation.Shares)
		// If the delegation is not dust, return an error and stop the donation flow
		if !removedTokens.IsZero() {
			return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s has a non-zero staking entry", msg.FromAddress)
		}

		// Remove the dust delegation shares from the validator
		err = sk.RemoveDelegation(ctx, delegation)
		if err != nil {
			return nil, err
		}
	}

	// check whether an account has any other type of staking entries
	if len(sk.GetUnbondingDelegations(ctx, acc.GetAddress(), 1)) != 0 ||
		len(sk.GetRedelegations(ctx, acc.GetAddress(), 1)) != 0 {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s has staking entry", msg.FromAddress)
	}

	vestingAcc, ok := acc.(exported.VestingAccount)
	if !ok {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s is not a vesting account", msg.FromAddress)
	}

	vestingCoins := vestingAcc.GetVestingCoins(ctx.BlockTime())
	if vestingCoins.IsZero() {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "account %s has no vesting tokens", msg.FromAddress)
	}

	// Change the account as normal account
	ak.SetAccount(ctx,
		authtypes.NewBaseAccount(
			acc.GetAddress(),
			acc.GetPubKey(),
			acc.GetAccountNumber(),
			acc.GetSequence(),
		),
	)

	if err := dk.FundCommunityPool(ctx, vestingCoins, from); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeyAmount, vestingCoins.String()),
		),
	)

	return &types.MsgDonateAllVestingTokensResponse{}, nil
}
