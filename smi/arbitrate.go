//
// (c) 2018 ReconfigureIO
//
// <COPYRIGHT TERMS>
//

//
// SMI protocol bus arbitration between multiple 'upstream' ports. This package
// specifies a set of goroutines which may be used to arbitrate between multiple
// upstream SMI memory ports and a single downstream port. The current
// implementation supports arbitration between 2, 3 or 4 64-bit upstream ports.
// TODO: Support arbitrary number of upstream ports on demand using the Go
// generate capability.
//

//
// Package arbitrate provides reusable arbitrators for SMI transactions.
//
package smi

//
// manageUpstreamPort provides transaction management for the arbitrated
// upstream ports. This includes header tag switching to allow request and
// response message pairs to be matched up.
//
func manageUpstreamPort(
	upstreamRequest <-chan Flit64,
	upstreamResponse chan<- Flit64,
	taggedRequest chan<- Flit64,
	taggedResponse <-chan Flit64,
	transferReq chan<- uint8,
	portId uint8) {

	// Split the tags into upper and lower bytes for efficient access.
	// TODO: The array and channel sizes here should be set using the
	// SmiMemInFlightLimit constant once supported by the compiler.
	var tagTableLower [4]uint8
	var tagTableUpper [4]uint8
	tagFifo := make(chan uint8, 4)

	// Set up the local tag values.
	for tagInit := uint8(0); tagInit != 4; tagInit++ {
		tagFifo <- tagInit
	}

	// Start goroutine for tag replacement on requests.
	go func() {
		for {

			// Do tag replacement on header.
			headerFlit := <-upstreamRequest
			tagId := <-tagFifo
			tagTableLower[tagId] = headerFlit.Data[2]
			tagTableUpper[tagId] = headerFlit.Data[3]
			headerFlit.Data[2] = portId
			headerFlit.Data[3] = tagId
			transferReq <- portId
			taggedRequest <- headerFlit

			// Copy remaining flits from upstream to downstream.
			moreFlits := headerFlit.Eofc == 0
			for moreFlits {
				bodyFlit := <-upstreamRequest
				moreFlits = bodyFlit.Eofc == 0
				taggedRequest <- bodyFlit
			}
		}
	}()

	// Carry out tag replacement on responses.
	for {

		// Extract tag ID from header and use it to look up replacement.
		headerFlit := <-taggedResponse
		tagId := headerFlit.Data[3]
		headerFlit.Data[2] = tagTableLower[tagId]
		headerFlit.Data[3] = tagTableUpper[tagId]
		tagFifo <- tagId
		upstreamResponse <- headerFlit

		// Copy remaining flits from downstream to upstream.
		moreFlits := headerFlit.Eofc == 0
		for moreFlits {
			bodyFlit := <-taggedResponse
			moreFlits = bodyFlit.Eofc == 0
			upstreamResponse <- bodyFlit
		}
	}
}

//
// ArbitrateX2 is a goroutine for providing arbitration between two pairs of
// SMI request/response channels. This uses tag matching and substitution on
// bytes 2 and 3 of each transfer to ensure that response frames are correctly
// routed to the source of the original request.
//
func ArbitrateX2(
	upstreamRequestA <-chan Flit64,
	upstreamResponseA chan<- Flit64,
	upstreamRequestB <-chan Flit64,
	upstreamResponseB chan<- Flit64,
	downstreamRequest chan<- Flit64,
	downstreamResponse <-chan Flit64) {

	// Define local channel connections.
	taggedRequestA := make(chan Flit64, 1)
	taggedResponseA := make(chan Flit64, 1)
	taggedRequestB := make(chan Flit64, 1)
	taggedResponseB := make(chan Flit64, 1)
	transferReqA := make(chan uint8, 1)
	transferReqB := make(chan uint8, 1)

	// Run the upstream port management routines.
	go manageUpstreamPort(upstreamRequestA, upstreamResponseA,
		taggedRequestA, taggedResponseA, transferReqA, uint8(1))
	go manageUpstreamPort(upstreamRequestB, upstreamResponseB,
		taggedRequestB, taggedResponseB, transferReqB, uint8(2))

	// Arbitrate between transfer requests.
	go func() {
		for {

			// Gets port ID of active input.
			var portId uint8
			select {
			case portId = <-transferReqA:
			case portId = <-transferReqB:
			}

			// Copy over input data.
			var reqFlit Flit64
			moreFlits := true
			for moreFlits {
				switch portId {
				case 1:
					reqFlit = <-taggedRequestA
				default:
					reqFlit = <-taggedRequestB
				}
				downstreamRequest <- reqFlit
				moreFlits = reqFlit.Eofc == 0
			}
		}
	}()

	// Steer transfer responses.
	portId := uint8(0)
	isHeaderFlit := true
	for {
		respFlit := <-downstreamResponse
		if isHeaderFlit {
			portId = respFlit.Data[2]
		}
		switch portId {
		case 1:
			taggedResponseA <- respFlit
		case 2:
			taggedResponseB <- respFlit
		default:
			// Discard invalid flit.
		}
		isHeaderFlit = respFlit.Eofc != 0
	}
}

//
// ArbitrateX3 is a goroutine for providing arbitration between three pairs of
// SMI request/response channels. This uses tag matching and substitution on
// bytes 2 and 3 of each transfer to ensure that response frames are correctly
// routed to the source of the original request.
//
func ArbitrateX3(
	upstreamRequestA <-chan Flit64,
	upstreamResponseA chan<- Flit64,
	upstreamRequestB <-chan Flit64,
	upstreamResponseB chan<- Flit64,
	upstreamRequestC <-chan Flit64,
	upstreamResponseC chan<- Flit64,
	downstreamRequest chan<- Flit64,
	downstreamResponse <-chan Flit64) {

	// Define local channel connections.
	taggedRequestA := make(chan Flit64, 1)
	taggedResponseA := make(chan Flit64, 1)
	taggedRequestB := make(chan Flit64, 1)
	taggedResponseB := make(chan Flit64, 1)
	taggedRequestC := make(chan Flit64, 1)
	taggedResponseC := make(chan Flit64, 1)
	transferReqA := make(chan uint8, 1)
	transferReqB := make(chan uint8, 1)
	transferReqC := make(chan uint8, 1)

	// Run the upstream port management routines.
	go manageUpstreamPort(upstreamRequestA, upstreamResponseA,
		taggedRequestA, taggedResponseA, transferReqA, uint8(1))
	go manageUpstreamPort(upstreamRequestB, upstreamResponseB,
		taggedRequestB, taggedResponseB, transferReqB, uint8(2))
	go manageUpstreamPort(upstreamRequestC, upstreamResponseC,
		taggedRequestC, taggedResponseC, transferReqC, uint8(3))

	// Arbitrate between transfer requests.
	go func() {
		for {

			// Gets port ID of active input.
			var portId uint8
			select {
			case portId = <-transferReqA:
			case portId = <-transferReqB:
			case portId = <-transferReqC:
			}

			// Copy over input data.
			var reqFlit Flit64
			moreFlits := true
			for moreFlits {
				switch portId {
				case 1:
					reqFlit = <-taggedRequestA
				case 2:
					reqFlit = <-taggedRequestB
				default:
					reqFlit = <-taggedRequestC
				}
				downstreamRequest <- reqFlit
				moreFlits = reqFlit.Eofc == 0
			}
		}
	}()

	// Steer transfer responses.
	portId := uint8(0)
	isHeaderFlit := true
	for {
		respFlit := <-downstreamResponse
		if isHeaderFlit {
			portId = respFlit.Data[2]
		}
		switch portId {
		case 1:
			taggedResponseA <- respFlit
		case 2:
			taggedResponseB <- respFlit
		case 3:
			taggedResponseC <- respFlit
		default:
			// Discard invalid flit.
		}
		isHeaderFlit = respFlit.Eofc != 0
	}
}

//
// ArbitrateX4 is a goroutine for providing arbitration between four pairs of
// SMI request/response channels. This uses tag matching and substitution on
// bytes 2 and 3 of each transfer to ensure that response frames are correctly
// routed to the source of the original request.
//
func ArbitrateX4(
	upstreamRequestA <-chan Flit64,
	upstreamResponseA chan<- Flit64,
	upstreamRequestB <-chan Flit64,
	upstreamResponseB chan<- Flit64,
	upstreamRequestC <-chan Flit64,
	upstreamResponseC chan<- Flit64,
	upstreamRequestD <-chan Flit64,
	upstreamResponseD chan<- Flit64,
	downstreamRequest chan<- Flit64,
	downstreamResponse <-chan Flit64) {

	// Define local channel connections.
	taggedRequestA := make(chan Flit64, 1)
	taggedResponseA := make(chan Flit64, 1)
	taggedRequestB := make(chan Flit64, 1)
	taggedResponseB := make(chan Flit64, 1)
	taggedRequestC := make(chan Flit64, 1)
	taggedResponseC := make(chan Flit64, 1)
	taggedRequestD := make(chan Flit64, 1)
	taggedResponseD := make(chan Flit64, 1)
	transferReqA := make(chan uint8, 1)
	transferReqB := make(chan uint8, 1)
	transferReqC := make(chan uint8, 1)
	transferReqD := make(chan uint8, 1)

	// Run the upstream port management routines.
	go manageUpstreamPort(upstreamRequestA, upstreamResponseA,
		taggedRequestA, taggedResponseA, transferReqA, uint8(1))
	go manageUpstreamPort(upstreamRequestB, upstreamResponseB,
		taggedRequestB, taggedResponseB, transferReqB, uint8(2))
	go manageUpstreamPort(upstreamRequestC, upstreamResponseC,
		taggedRequestC, taggedResponseC, transferReqC, uint8(3))
	go manageUpstreamPort(upstreamRequestD, upstreamResponseD,
		taggedRequestD, taggedResponseD, transferReqD, uint8(4))

	// Arbitrate between transfer requests.
	go func() {
		for {

			// Gets port ID of active input.
			var portId uint8
			select {
			case portId = <-transferReqA:
			case portId = <-transferReqB:
			case portId = <-transferReqC:
			case portId = <-transferReqD:
			}

			// Copy over input data.
			var reqFlit Flit64
			moreFlits := true
			for moreFlits {
				switch portId {
				case 1:
					reqFlit = <-taggedRequestA
				case 2:
					reqFlit = <-taggedRequestB
				case 3:
					reqFlit = <-taggedRequestC
				default:
					reqFlit = <-taggedRequestD
				}
				downstreamRequest <- reqFlit
				moreFlits = reqFlit.Eofc == 0
			}
		}
	}()

	// Steer transfer responses.
	portId := uint8(0)
	isHeaderFlit := true
	for {
		respFlit := <-downstreamResponse
		if isHeaderFlit {
			portId = respFlit.Data[2]
		}
		switch portId {
		case 1:
			taggedResponseA <- respFlit
		case 2:
			taggedResponseB <- respFlit
		case 3:
			taggedResponseC <- respFlit
		case 4:
			taggedResponseD <- respFlit
		default:
			// Discard invalid flit.
		}
		isHeaderFlit = respFlit.Eofc != 0
	}
}
