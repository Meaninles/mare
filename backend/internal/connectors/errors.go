package connectors

import "fmt"

type ErrorCode string

const (
	ErrorCodeNotFound       ErrorCode = "not_found"
	ErrorCodeAccessDenied   ErrorCode = "access_denied"
	ErrorCodeUnavailable    ErrorCode = "unavailable"
	ErrorCodeNotSupported   ErrorCode = "not_supported"
	ErrorCodeInvalidConfig  ErrorCode = "invalid_config"
	ErrorCodeAuthentication ErrorCode = "authentication_failed"
)

type ConnectorError struct {
	Code       ErrorCode
	Connector  EndpointType
	Operation  string
	Message    string
	Temporary  bool
	Underlying error
}

func (err *ConnectorError) Error() string {
	if err.Message != "" {
		return fmt.Sprintf("%s %s: %s", err.Connector, err.Operation, err.Message)
	}
	if err.Underlying != nil {
		return fmt.Sprintf("%s %s: %v", err.Connector, err.Operation, err.Underlying)
	}
	return fmt.Sprintf("%s %s failed", err.Connector, err.Operation)
}

func (err *ConnectorError) Unwrap() error {
	return err.Underlying
}

func newConnectorError(connector EndpointType, operation string, code ErrorCode, message string, temporary bool, underlying error) error {
	return &ConnectorError{
		Code:       code,
		Connector:  connector,
		Operation:  operation,
		Message:    message,
		Temporary:  temporary,
		Underlying: underlying,
	}
}
