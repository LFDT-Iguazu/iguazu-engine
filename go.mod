module github.com/LFDT-Iguazu/iguazu-engine

go 1.21

require (
	github.com/LFDT-Iguazu/iguazu-types v0.1.0
	github.com/holiman/uint256 v1.2.4
	github.com/minio/sha256-simd v1.0.0
	github.com/prysmaticlabs/go-bitfield v0.0.0-20210809151128-385d8c5e3fb7
	github.com/stretchr/testify v1.11.1
	github.com/waterfall-network/fastssz v0.1.4
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/supranational/blst v0.3.16 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Superseded by v0.2.0.
retract v0.1.0

replace github.com/LFDT-Iguazu/iguazu-types => ../wf-types
