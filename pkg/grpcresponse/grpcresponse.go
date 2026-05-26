package grpcresponse

import "google.golang.org/grpc/codes"

type Response struct {
	Code codes.Code `json:"code"`
	Data any        `json:"data,omitempty"`
}

func New(code codes.Code, data any) *Response {
	return &Response{
		Code: code,
		Data: data,
	}
}
