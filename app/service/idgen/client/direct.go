package idgen_client

// NewIDGenClient2Direct points sequence calls at an IdgenClient that is
// already in this process. NewIDGenClient2 dials gRPC and cannot reach that core.
func NewIDGenClient2Direct(cli IdgenClient) IDGenClient2 {
	return IDGenClient2{cli: cli}
}
