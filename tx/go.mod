module cosmossdk.io/tx

go 1.26.5

require (
	cosmossdk.io/api v1.1.0
	cosmossdk.io/core v0.3.2
	cosmossdk.io/math v1.3.0
	github.com/cosmos/cosmos-proto v1.0.0-beta.5
	github.com/stretchr/testify v1.9.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/cosmos/gogoproto v1.7.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/exp v0.0.0-20230711153332-06a737ee72cb // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// temporary until we tag a new go module
replace cosmossdk.io/core => ../core
