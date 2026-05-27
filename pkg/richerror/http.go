package richerror

import (
	"errors"
	"net/http"
)

type ErrHTTPResponse struct {
	Message     string              `json:"message"`
	Code        int                 `json:"-"`
	Validations map[string][]string `json:"errors,omitempty"`
}

var kindToHTTPStatus = map[Kind]int{
	KindUnknown:         http.StatusInternalServerError,
	KindBadRequest:      http.StatusBadRequest,
	KindUnauthorized:    http.StatusUnauthorized,
	KindForbidden:       http.StatusForbidden,
	KindNotFound:        http.StatusNotFound,
	KindConflict:        http.StatusConflict,
	KindGone:            http.StatusGone,
	KindTooManyRequests: http.StatusTooManyRequests,
	KindInternal:        http.StatusInternalServerError,
	KindUnavailable:     http.StatusServiceUnavailable,
	KindUnprocessable:   http.StatusUnprocessableEntity,
}

func (r *RichError) ToHTTPStatus() int {
	status, ok := kindToHTTPStatus[r.Kind()]
	if !ok {
		return http.StatusInternalServerError
	}
	return status
}

func ErrHTTP(err error) *ErrHTTPResponse {
	var re *RichError
	if errors.As(err, &re) {
		return &ErrHTTPResponse{
			Message:     re.Message(),
			Code:        re.ToHTTPStatus(),
			Validations: re.Validations(),
		}
	}

	return &ErrHTTPResponse{
		Message: err.Error(),
		Code:    http.StatusInternalServerError,
	}
}
