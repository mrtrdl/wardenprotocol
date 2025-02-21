package client

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/warden-protocol/wardenprotocol/warden/x/warden/types"
)

type KeyRequestFulfilment struct {
	RequestID uint64
	PublicKey []byte
}

func (r KeyRequestFulfilment) Msg(creator string) sdk.Msg {
	return &types.MsgUpdateKeyRequest{
		Creator:   creator,
		RequestId: r.RequestID,
		Status:    types.KeyRequestStatus_KEY_REQUEST_STATUS_FULFILLED,
		Result:    types.NewMsgUpdateKeyRequestKey(r.PublicKey),
	}
}

type KeyRequestRejection struct {
	RequestID uint64
	Reason    string
}

func (r KeyRequestRejection) Msg(creator string) sdk.Msg {
	return &types.MsgUpdateKeyRequest{
		Creator:   creator,
		RequestId: r.RequestID,
		Status:    types.KeyRequestStatus_KEY_REQUEST_STATUS_REJECTED,
		Result:    types.NewMsgUpdateKeyRequestReject(r.Reason),
	}
}

type SignRequestFulfilment struct {
	RequestID uint64
	Signature []byte
}

func (r SignRequestFulfilment) Msg(creator string) sdk.Msg {
	return &types.MsgFulfilSignatureRequest{
		Creator:   creator,
		RequestId: r.RequestID,
		Status:    types.SignRequestStatus_SIGN_REQUEST_STATUS_FULFILLED,
		Result: &types.MsgFulfilSignatureRequest_Payload{
			Payload: &types.MsgSignedData{
				SignedData: r.Signature,
			},
		},
	}
}

type SignRequestRejection struct {
	RequestID uint64
	Reason    string
}

func (r SignRequestRejection) Msg(creator string) sdk.Msg {
	return &types.MsgFulfilSignatureRequest{
		Creator:   creator,
		RequestId: r.RequestID,
		Status:    types.SignRequestStatus_SIGN_REQUEST_STATUS_REJECTED,
		Result: &types.MsgFulfilSignatureRequest_RejectReason{
			RejectReason: r.Reason,
		},
	}
}
