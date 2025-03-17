module cosmossdk.io/tx

go 1.19
toolchain go1.24.1

require (
	cosmossdk.io/api v0.3.1
	cosmossdk.io/core v0.3.2
	cosmossdk.io/math v1.5.0
	github.com/cosmos/cosmos-proto v1.0.0-beta.4
	github.com/stretchr/testify v1.10.0
	google.golang.org/protobuf v1.36.1
)

require (
	cosmossdk.io/errors v1.0.1 // indirect
	github.com/cockroachdb/apd/v3 v3.2.1 // indirect
	github.com/cosmos/gogoproto v1.4.10 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/exp v0.0.0-20240404231335-c0f41cb1a7a0 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	google.golang.org/grpc v1.64.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// temporary until we tag a new go module
replace cosmossdk.io/core => ../core
