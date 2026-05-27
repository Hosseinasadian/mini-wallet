package httpresponse

type Response struct {
	Code int `json:"code"`
	Data any `json:"data,omitempty"`
}

type IsReady struct {
	IsReady bool   `json:"is_ready"`
	Reason  string `json:"reason,omitempty"`
}

func New(code int, data any) *Response {
	return &Response{
		Code: code,
		Data: data,
	}
}
