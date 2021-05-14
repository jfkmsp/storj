// Code generated by protoc-gen-go-drpc. DO NOT EDIT.
// protoc-gen-go-drpc version: v0.0.20
// source: multinode.proto

package multinodepb

import (
	bytes "bytes"
	context "context"
	errors "errors"

	jsonpb "github.com/gogo/protobuf/jsonpb"
	proto "github.com/gogo/protobuf/proto"

	drpc "storj.io/drpc"
	drpcerr "storj.io/drpc/drpcerr"
)

type drpcEncoding_File_multinode_proto struct{}

func (drpcEncoding_File_multinode_proto) Marshal(msg drpc.Message) ([]byte, error) {
	return proto.Marshal(msg.(proto.Message))
}

func (drpcEncoding_File_multinode_proto) Unmarshal(buf []byte, msg drpc.Message) error {
	return proto.Unmarshal(buf, msg.(proto.Message))
}

func (drpcEncoding_File_multinode_proto) JSONMarshal(msg drpc.Message) ([]byte, error) {
	var buf bytes.Buffer
	err := new(jsonpb.Marshaler).Marshal(&buf, msg.(proto.Message))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (drpcEncoding_File_multinode_proto) JSONUnmarshal(buf []byte, msg drpc.Message) error {
	return jsonpb.Unmarshal(bytes.NewReader(buf), msg.(proto.Message))
}

type DRPCStorageClient interface {
	DRPCConn() drpc.Conn

	DiskSpace(ctx context.Context, in *DiskSpaceRequest) (*DiskSpaceResponse, error)
}

type drpcStorageClient struct {
	cc drpc.Conn
}

func NewDRPCStorageClient(cc drpc.Conn) DRPCStorageClient {
	return &drpcStorageClient{cc}
}

func (c *drpcStorageClient) DRPCConn() drpc.Conn { return c.cc }

func (c *drpcStorageClient) DiskSpace(ctx context.Context, in *DiskSpaceRequest) (*DiskSpaceResponse, error) {
	out := new(DiskSpaceResponse)
	err := c.cc.Invoke(ctx, "/multinode.Storage/DiskSpace", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type DRPCStorageServer interface {
	DiskSpace(context.Context, *DiskSpaceRequest) (*DiskSpaceResponse, error)
}

type DRPCStorageUnimplementedServer struct{}

func (s *DRPCStorageUnimplementedServer) DiskSpace(context.Context, *DiskSpaceRequest) (*DiskSpaceResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

type DRPCStorageDescription struct{}

func (DRPCStorageDescription) NumMethods() int { return 1 }

func (DRPCStorageDescription) Method(n int) (string, drpc.Encoding, drpc.Receiver, interface{}, bool) {
	switch n {
	case 0:
		return "/multinode.Storage/DiskSpace", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCStorageServer).
					DiskSpace(
						ctx,
						in1.(*DiskSpaceRequest),
					)
			}, DRPCStorageServer.DiskSpace, true
	default:
		return "", nil, nil, nil, false
	}
}

func DRPCRegisterStorage(mux drpc.Mux, impl DRPCStorageServer) error {
	return mux.Register(impl, DRPCStorageDescription{})
}

type DRPCStorage_DiskSpaceStream interface {
	drpc.Stream
	SendAndClose(*DiskSpaceResponse) error
}

type drpcStorage_DiskSpaceStream struct {
	drpc.Stream
}

func (x *drpcStorage_DiskSpaceStream) SendAndClose(m *DiskSpaceResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCBandwidthClient interface {
	DRPCConn() drpc.Conn

	MonthSummary(ctx context.Context, in *BandwidthMonthSummaryRequest) (*BandwidthMonthSummaryResponse, error)
}

type drpcBandwidthClient struct {
	cc drpc.Conn
}

func NewDRPCBandwidthClient(cc drpc.Conn) DRPCBandwidthClient {
	return &drpcBandwidthClient{cc}
}

func (c *drpcBandwidthClient) DRPCConn() drpc.Conn { return c.cc }

func (c *drpcBandwidthClient) MonthSummary(ctx context.Context, in *BandwidthMonthSummaryRequest) (*BandwidthMonthSummaryResponse, error) {
	out := new(BandwidthMonthSummaryResponse)
	err := c.cc.Invoke(ctx, "/multinode.Bandwidth/MonthSummary", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type DRPCBandwidthServer interface {
	MonthSummary(context.Context, *BandwidthMonthSummaryRequest) (*BandwidthMonthSummaryResponse, error)
}

type DRPCBandwidthUnimplementedServer struct{}

func (s *DRPCBandwidthUnimplementedServer) MonthSummary(context.Context, *BandwidthMonthSummaryRequest) (*BandwidthMonthSummaryResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

type DRPCBandwidthDescription struct{}

func (DRPCBandwidthDescription) NumMethods() int { return 1 }

func (DRPCBandwidthDescription) Method(n int) (string, drpc.Encoding, drpc.Receiver, interface{}, bool) {
	switch n {
	case 0:
		return "/multinode.Bandwidth/MonthSummary", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCBandwidthServer).
					MonthSummary(
						ctx,
						in1.(*BandwidthMonthSummaryRequest),
					)
			}, DRPCBandwidthServer.MonthSummary, true
	default:
		return "", nil, nil, nil, false
	}
}

func DRPCRegisterBandwidth(mux drpc.Mux, impl DRPCBandwidthServer) error {
	return mux.Register(impl, DRPCBandwidthDescription{})
}

type DRPCBandwidth_MonthSummaryStream interface {
	drpc.Stream
	SendAndClose(*BandwidthMonthSummaryResponse) error
}

type drpcBandwidth_MonthSummaryStream struct {
	drpc.Stream
}

func (x *drpcBandwidth_MonthSummaryStream) SendAndClose(m *BandwidthMonthSummaryResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCNodeClient interface {
	DRPCConn() drpc.Conn

	Version(ctx context.Context, in *VersionRequest) (*VersionResponse, error)
	LastContact(ctx context.Context, in *LastContactRequest) (*LastContactResponse, error)
	Reputation(ctx context.Context, in *ReputationRequest) (*ReputationResponse, error)
	TrustedSatellites(ctx context.Context, in *TrustedSatellitesRequest) (*TrustedSatellitesResponse, error)
}

type drpcNodeClient struct {
	cc drpc.Conn
}

func NewDRPCNodeClient(cc drpc.Conn) DRPCNodeClient {
	return &drpcNodeClient{cc}
}

func (c *drpcNodeClient) DRPCConn() drpc.Conn { return c.cc }

func (c *drpcNodeClient) Version(ctx context.Context, in *VersionRequest) (*VersionResponse, error) {
	out := new(VersionResponse)
	err := c.cc.Invoke(ctx, "/multinode.Node/Version", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcNodeClient) LastContact(ctx context.Context, in *LastContactRequest) (*LastContactResponse, error) {
	out := new(LastContactResponse)
	err := c.cc.Invoke(ctx, "/multinode.Node/LastContact", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcNodeClient) Reputation(ctx context.Context, in *ReputationRequest) (*ReputationResponse, error) {
	out := new(ReputationResponse)
	err := c.cc.Invoke(ctx, "/multinode.Node/Reputation", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcNodeClient) TrustedSatellites(ctx context.Context, in *TrustedSatellitesRequest) (*TrustedSatellitesResponse, error) {
	out := new(TrustedSatellitesResponse)
	err := c.cc.Invoke(ctx, "/multinode.Node/TrustedSatellites", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type DRPCNodeServer interface {
	Version(context.Context, *VersionRequest) (*VersionResponse, error)
	LastContact(context.Context, *LastContactRequest) (*LastContactResponse, error)
	Reputation(context.Context, *ReputationRequest) (*ReputationResponse, error)
	TrustedSatellites(context.Context, *TrustedSatellitesRequest) (*TrustedSatellitesResponse, error)
}

type DRPCNodeUnimplementedServer struct{}

func (s *DRPCNodeUnimplementedServer) Version(context.Context, *VersionRequest) (*VersionResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCNodeUnimplementedServer) LastContact(context.Context, *LastContactRequest) (*LastContactResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCNodeUnimplementedServer) Reputation(context.Context, *ReputationRequest) (*ReputationResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCNodeUnimplementedServer) TrustedSatellites(context.Context, *TrustedSatellitesRequest) (*TrustedSatellitesResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

type DRPCNodeDescription struct{}

func (DRPCNodeDescription) NumMethods() int { return 4 }

func (DRPCNodeDescription) Method(n int) (string, drpc.Encoding, drpc.Receiver, interface{}, bool) {
	switch n {
	case 0:
		return "/multinode.Node/Version", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCNodeServer).
					Version(
						ctx,
						in1.(*VersionRequest),
					)
			}, DRPCNodeServer.Version, true
	case 1:
		return "/multinode.Node/LastContact", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCNodeServer).
					LastContact(
						ctx,
						in1.(*LastContactRequest),
					)
			}, DRPCNodeServer.LastContact, true
	case 2:
		return "/multinode.Node/Reputation", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCNodeServer).
					Reputation(
						ctx,
						in1.(*ReputationRequest),
					)
			}, DRPCNodeServer.Reputation, true
	case 3:
		return "/multinode.Node/TrustedSatellites", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCNodeServer).
					TrustedSatellites(
						ctx,
						in1.(*TrustedSatellitesRequest),
					)
			}, DRPCNodeServer.TrustedSatellites, true
	default:
		return "", nil, nil, nil, false
	}
}

func DRPCRegisterNode(mux drpc.Mux, impl DRPCNodeServer) error {
	return mux.Register(impl, DRPCNodeDescription{})
}

type DRPCNode_VersionStream interface {
	drpc.Stream
	SendAndClose(*VersionResponse) error
}

type drpcNode_VersionStream struct {
	drpc.Stream
}

func (x *drpcNode_VersionStream) SendAndClose(m *VersionResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCNode_LastContactStream interface {
	drpc.Stream
	SendAndClose(*LastContactResponse) error
}

type drpcNode_LastContactStream struct {
	drpc.Stream
}

func (x *drpcNode_LastContactStream) SendAndClose(m *LastContactResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCNode_ReputationStream interface {
	drpc.Stream
	SendAndClose(*ReputationResponse) error
}

type drpcNode_ReputationStream struct {
	drpc.Stream
}

func (x *drpcNode_ReputationStream) SendAndClose(m *ReputationResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCNode_TrustedSatellitesStream interface {
	drpc.Stream
	SendAndClose(*TrustedSatellitesResponse) error
}

type drpcNode_TrustedSatellitesStream struct {
	drpc.Stream
}

func (x *drpcNode_TrustedSatellitesStream) SendAndClose(m *TrustedSatellitesResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayoutClient interface {
	DRPCConn() drpc.Conn

	AllSatellitesSummary(ctx context.Context, in *AllSatellitesSummaryRequest) (*AllSatellitesSummaryResponse, error)
	AllSatellitesPeriodSummary(ctx context.Context, in *AllSatellitesPeriodSummaryRequest) (*AllSatellitesPeriodSummaryResponse, error)
	SatelliteSummary(ctx context.Context, in *SatelliteSummaryRequest) (*SatelliteSummaryResponse, error)
	SatellitePeriodSummary(ctx context.Context, in *SatellitePeriodSummaryRequest) (*SatellitePeriodSummaryResponse, error)
	Earned(ctx context.Context, in *EarnedRequest) (*EarnedResponse, error)
	EarnedPerSatellite(ctx context.Context, in *EarnedPerSatelliteRequest) (*EarnedPerSatelliteResponse, error)
	EstimatedPayoutSatellite(ctx context.Context, in *EstimatedPayoutSatelliteRequest) (*EstimatedPayoutSatelliteResponse, error)
	EstimatedPayoutTotal(ctx context.Context, in *EstimatedPayoutTotalRequest) (*EstimatedPayoutTotalResponse, error)
	Undistributed(ctx context.Context, in *UndistributedRequest) (*UndistributedResponse, error)
}

type drpcPayoutClient struct {
	cc drpc.Conn
}

func NewDRPCPayoutClient(cc drpc.Conn) DRPCPayoutClient {
	return &drpcPayoutClient{cc}
}

func (c *drpcPayoutClient) DRPCConn() drpc.Conn { return c.cc }

func (c *drpcPayoutClient) AllSatellitesSummary(ctx context.Context, in *AllSatellitesSummaryRequest) (*AllSatellitesSummaryResponse, error) {
	out := new(AllSatellitesSummaryResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/AllSatellitesSummary", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) AllSatellitesPeriodSummary(ctx context.Context, in *AllSatellitesPeriodSummaryRequest) (*AllSatellitesPeriodSummaryResponse, error) {
	out := new(AllSatellitesPeriodSummaryResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/AllSatellitesPeriodSummary", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) SatelliteSummary(ctx context.Context, in *SatelliteSummaryRequest) (*SatelliteSummaryResponse, error) {
	out := new(SatelliteSummaryResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/SatelliteSummary", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) SatellitePeriodSummary(ctx context.Context, in *SatellitePeriodSummaryRequest) (*SatellitePeriodSummaryResponse, error) {
	out := new(SatellitePeriodSummaryResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/SatellitePeriodSummary", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) Earned(ctx context.Context, in *EarnedRequest) (*EarnedResponse, error) {
	out := new(EarnedResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/Earned", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) EarnedPerSatellite(ctx context.Context, in *EarnedPerSatelliteRequest) (*EarnedPerSatelliteResponse, error) {
	out := new(EarnedPerSatelliteResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/EarnedPerSatellite", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) EstimatedPayoutSatellite(ctx context.Context, in *EstimatedPayoutSatelliteRequest) (*EstimatedPayoutSatelliteResponse, error) {
	out := new(EstimatedPayoutSatelliteResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/EstimatedPayoutSatellite", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) EstimatedPayoutTotal(ctx context.Context, in *EstimatedPayoutTotalRequest) (*EstimatedPayoutTotalResponse, error) {
	out := new(EstimatedPayoutTotalResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/EstimatedPayoutTotal", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcPayoutClient) Undistributed(ctx context.Context, in *UndistributedRequest) (*UndistributedResponse, error) {
	out := new(UndistributedResponse)
	err := c.cc.Invoke(ctx, "/multinode.Payout/Undistributed", drpcEncoding_File_multinode_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type DRPCPayoutServer interface {
	AllSatellitesSummary(context.Context, *AllSatellitesSummaryRequest) (*AllSatellitesSummaryResponse, error)
	AllSatellitesPeriodSummary(context.Context, *AllSatellitesPeriodSummaryRequest) (*AllSatellitesPeriodSummaryResponse, error)
	SatelliteSummary(context.Context, *SatelliteSummaryRequest) (*SatelliteSummaryResponse, error)
	SatellitePeriodSummary(context.Context, *SatellitePeriodSummaryRequest) (*SatellitePeriodSummaryResponse, error)
	Earned(context.Context, *EarnedRequest) (*EarnedResponse, error)
	EarnedPerSatellite(context.Context, *EarnedPerSatelliteRequest) (*EarnedPerSatelliteResponse, error)
	EstimatedPayoutSatellite(context.Context, *EstimatedPayoutSatelliteRequest) (*EstimatedPayoutSatelliteResponse, error)
	EstimatedPayoutTotal(context.Context, *EstimatedPayoutTotalRequest) (*EstimatedPayoutTotalResponse, error)
	Undistributed(context.Context, *UndistributedRequest) (*UndistributedResponse, error)
}

type DRPCPayoutUnimplementedServer struct{}

func (s *DRPCPayoutUnimplementedServer) AllSatellitesSummary(context.Context, *AllSatellitesSummaryRequest) (*AllSatellitesSummaryResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) AllSatellitesPeriodSummary(context.Context, *AllSatellitesPeriodSummaryRequest) (*AllSatellitesPeriodSummaryResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) SatelliteSummary(context.Context, *SatelliteSummaryRequest) (*SatelliteSummaryResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) SatellitePeriodSummary(context.Context, *SatellitePeriodSummaryRequest) (*SatellitePeriodSummaryResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) Earned(context.Context, *EarnedRequest) (*EarnedResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) EarnedPerSatellite(context.Context, *EarnedPerSatelliteRequest) (*EarnedPerSatelliteResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) EstimatedPayoutSatellite(context.Context, *EstimatedPayoutSatelliteRequest) (*EstimatedPayoutSatelliteResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) EstimatedPayoutTotal(context.Context, *EstimatedPayoutTotalRequest) (*EstimatedPayoutTotalResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

func (s *DRPCPayoutUnimplementedServer) Undistributed(context.Context, *UndistributedRequest) (*UndistributedResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), 12)
}

type DRPCPayoutDescription struct{}

func (DRPCPayoutDescription) NumMethods() int { return 9 }

func (DRPCPayoutDescription) Method(n int) (string, drpc.Encoding, drpc.Receiver, interface{}, bool) {
	switch n {
	case 0:
		return "/multinode.Payout/AllSatellitesSummary", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					AllSatellitesSummary(
						ctx,
						in1.(*AllSatellitesSummaryRequest),
					)
			}, DRPCPayoutServer.AllSatellitesSummary, true
	case 1:
		return "/multinode.Payout/AllSatellitesPeriodSummary", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					AllSatellitesPeriodSummary(
						ctx,
						in1.(*AllSatellitesPeriodSummaryRequest),
					)
			}, DRPCPayoutServer.AllSatellitesPeriodSummary, true
	case 2:
		return "/multinode.Payout/SatelliteSummary", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					SatelliteSummary(
						ctx,
						in1.(*SatelliteSummaryRequest),
					)
			}, DRPCPayoutServer.SatelliteSummary, true
	case 3:
		return "/multinode.Payout/SatellitePeriodSummary", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					SatellitePeriodSummary(
						ctx,
						in1.(*SatellitePeriodSummaryRequest),
					)
			}, DRPCPayoutServer.SatellitePeriodSummary, true
	case 4:
		return "/multinode.Payout/Earned", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					Earned(
						ctx,
						in1.(*EarnedRequest),
					)
			}, DRPCPayoutServer.Earned, true
	case 5:
		return "/multinode.Payout/EarnedPerSatellite", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					EarnedPerSatellite(
						ctx,
						in1.(*EarnedPerSatelliteRequest),
					)
			}, DRPCPayoutServer.EarnedPerSatellite, true
	case 6:
		return "/multinode.Payout/EstimatedPayoutSatellite", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					EstimatedPayoutSatellite(
						ctx,
						in1.(*EstimatedPayoutSatelliteRequest),
					)
			}, DRPCPayoutServer.EstimatedPayoutSatellite, true
	case 7:
		return "/multinode.Payout/EstimatedPayoutTotal", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					EstimatedPayoutTotal(
						ctx,
						in1.(*EstimatedPayoutTotalRequest),
					)
			}, DRPCPayoutServer.EstimatedPayoutTotal, true
	case 8:
		return "/multinode.Payout/Undistributed", drpcEncoding_File_multinode_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCPayoutServer).
					Undistributed(
						ctx,
						in1.(*UndistributedRequest),
					)
			}, DRPCPayoutServer.Undistributed, true
	default:
		return "", nil, nil, nil, false
	}
}

func DRPCRegisterPayout(mux drpc.Mux, impl DRPCPayoutServer) error {
	return mux.Register(impl, DRPCPayoutDescription{})
}

type DRPCPayout_AllSatellitesSummaryStream interface {
	drpc.Stream
	SendAndClose(*AllSatellitesSummaryResponse) error
}

type drpcPayout_AllSatellitesSummaryStream struct {
	drpc.Stream
}

func (x *drpcPayout_AllSatellitesSummaryStream) SendAndClose(m *AllSatellitesSummaryResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_AllSatellitesPeriodSummaryStream interface {
	drpc.Stream
	SendAndClose(*AllSatellitesPeriodSummaryResponse) error
}

type drpcPayout_AllSatellitesPeriodSummaryStream struct {
	drpc.Stream
}

func (x *drpcPayout_AllSatellitesPeriodSummaryStream) SendAndClose(m *AllSatellitesPeriodSummaryResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_SatelliteSummaryStream interface {
	drpc.Stream
	SendAndClose(*SatelliteSummaryResponse) error
}

type drpcPayout_SatelliteSummaryStream struct {
	drpc.Stream
}

func (x *drpcPayout_SatelliteSummaryStream) SendAndClose(m *SatelliteSummaryResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_SatellitePeriodSummaryStream interface {
	drpc.Stream
	SendAndClose(*SatellitePeriodSummaryResponse) error
}

type drpcPayout_SatellitePeriodSummaryStream struct {
	drpc.Stream
}

func (x *drpcPayout_SatellitePeriodSummaryStream) SendAndClose(m *SatellitePeriodSummaryResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_EarnedStream interface {
	drpc.Stream
	SendAndClose(*EarnedResponse) error
}

type drpcPayout_EarnedStream struct {
	drpc.Stream
}

func (x *drpcPayout_EarnedStream) SendAndClose(m *EarnedResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_EarnedPerSatelliteStream interface {
	drpc.Stream
	SendAndClose(*EarnedPerSatelliteResponse) error
}

type drpcPayout_EarnedPerSatelliteStream struct {
	drpc.Stream
}

func (x *drpcPayout_EarnedPerSatelliteStream) SendAndClose(m *EarnedPerSatelliteResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_EstimatedPayoutSatelliteStream interface {
	drpc.Stream
	SendAndClose(*EstimatedPayoutSatelliteResponse) error
}

type drpcPayout_EstimatedPayoutSatelliteStream struct {
	drpc.Stream
}

func (x *drpcPayout_EstimatedPayoutSatelliteStream) SendAndClose(m *EstimatedPayoutSatelliteResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_EstimatedPayoutTotalStream interface {
	drpc.Stream
	SendAndClose(*EstimatedPayoutTotalResponse) error
}

type drpcPayout_EstimatedPayoutTotalStream struct {
	drpc.Stream
}

func (x *drpcPayout_EstimatedPayoutTotalStream) SendAndClose(m *EstimatedPayoutTotalResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCPayout_UndistributedStream interface {
	drpc.Stream
	SendAndClose(*UndistributedResponse) error
}

type drpcPayout_UndistributedStream struct {
	drpc.Stream
}

func (x *drpcPayout_UndistributedStream) SendAndClose(m *UndistributedResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_multinode_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}
