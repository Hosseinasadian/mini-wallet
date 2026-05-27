package richerror

import (
	"errors"
)

type Kind int

const (
	KindUnknown Kind = iota
	KindBadRequest
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindGone
	KindTooManyRequests
	KindInternal
	KindUnavailable
	KindUnprocessable
)

type Operation string

func (op Operation) String() string {
	return string(op)
}

type RichError struct {
	operations  []Operation
	wrapper     error
	message     string
	kind        Kind
	validations map[string][]string
}

func New(operation Operation) *RichError {
	return &RichError{operations: []Operation{operation}}
}

func (r *RichError) WithWrapper(wrapper error) *RichError {
	r.wrapper = wrapper
	var richErr *RichError
	if errors.As(wrapper, &richErr) {
		r.operations = append(r.operations, richErr.operations...)
	}
	return r
}

func (r *RichError) WithMessage(message string) *RichError {
	r.message = message
	return r
}

func (r *RichError) WithKind(kind Kind) *RichError {
	r.kind = kind
	return r
}

func (r *RichError) WithValidations(validations map[string][]string) *RichError {
	r.validations = validations
	return r
}

func (r *RichError) WithValidation(field, message string) *RichError {
	if r.validations == nil {
		r.validations = make(map[string][]string)
	}
	r.validations[field] = append(r.validations[field], message)
	return r
}

func (r *RichError) Unwrap() error {
	return r.wrapper
}

func (r *RichError) Error() string {
	return r.message
}

func (r *RichError) Message() string {
	if r.message != "" {
		return r.message
	}

	var re *RichError
	if errors.As(r.wrapper, &re) {
		return re.Message()
	}

	if r.wrapper != nil {
		return r.wrapper.Error()
	}

	return ""
}

func (r *RichError) Kind() Kind {
	if r.kind != 0 {
		return r.kind
	}

	var re *RichError
	if errors.As(r.wrapper, &re) {
		return re.Kind()
	}

	return 0
}

func (r *RichError) Validations() map[string][]string {
	return r.validations
}

func (r *RichError) HasValidations() bool {
	return len(r.validations) > 0
}

func (r *RichError) Operations() []Operation {
	return r.operations
}

func UnwrapAll(err error) []string {
	var messages []string
	for err != nil {
		messages = append(messages, err.Error())
		err = errors.Unwrap(err)
	}
	return messages
}

func (r *RichError) Trace() ([]Operation, []string) {
	operations := r.Operations()

	if len(operations) > 0 {
		return operations[1:], UnwrapAll(r.Unwrap())
	}

	return nil, nil
}
