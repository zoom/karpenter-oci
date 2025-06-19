# Testing Directory

These test scenarios are designed to be used against a live OKE cluster running karpenter, and validate particular E2E scenarios.
before running the test case, please change the below vars to yours in test/pkg/environment/oci/environment.go file
```go
var clusterName
var clusterEndPoint
var compId
var clusterDns

var vcnId
var subnetName
var sgName
```

## File Directory
- `/suites`: Ginkgo test suites for particular scenarios live here.
- `/pkg`: Common code re-used across test suites lives here.
