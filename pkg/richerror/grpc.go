package richerror

import (
	"errors"
	"google.golang.org/grpc/codes"
)

type ErrGRPCResponse struct {
	Message     string              `json:"message"`
	Code        codes.Code          `json:"code"`
	Validations map[string][]string `json:"validations,omitempty"`
}

var kindToGRPCCode = map[Kind]codes.Code{
	KindUnknown:         codes.Internal,
	KindBadRequest:      codes.InvalidArgument,
	KindUnauthorized:    codes.Unauthenticated,
	KindForbidden:       codes.PermissionDenied,
	KindNotFound:        codes.NotFound,
	KindConflict:        codes.AlreadyExists,
	KindGone:            codes.NotFound,
	KindTooManyRequests: codes.ResourceExhausted,
	KindInternal:        codes.Internal,
	KindUnavailable:     codes.Unavailable,
	KindUnprocessable:   codes.InvalidArgument,
}

func (r *RichError) ToGRPCCode() codes.Code {
	code, ok := kindToGRPCCode[r.Kind()]
	if !ok {
		return codes.Internal
	}
	return code
}

func ErrGRPC(err error) *ErrGRPCResponse {
	var re *RichError
	if errors.As(err, &re) {
		return &ErrGRPCResponse{
			Message:     re.Message(),
			Code:        re.ToGRPCCode(),
			Validations: re.Validations(),
		}
	}

	return &ErrGRPCResponse{
		Message: err.Error(),
		Code:    codes.Internal,
	}
}
