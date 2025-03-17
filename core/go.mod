module cosmossdk.io/core

go 1.19
toolchain go1.24.1

require (
	cosmossdk.io/api v0.3.1
	cosmossdk.io/depinject v1.1.0
	cosmossdk.io/math v1.3.0
	github.com/cosmos/cosmos-proto v1.0.0-beta.5
	github.com/stretchr/testify v1.9.0
	google.golang.org/grpc v1.67.1
	google.golang.org/protobuf v1.35.1
	gotest.tools/v3 v3.5.1
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/cosmos/gogoproto v1.7.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// temporary until we tag a new go module
replace cosmossdk.io/math => ../math
