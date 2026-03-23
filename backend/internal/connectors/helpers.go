package connectors

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func remapConnectorType(err error, endpointType EndpointType) error {
	if err == nil {
		return nil
	}

	connectorErr, ok := err.(*ConnectorError)
	if !ok {
		return err
	}

	clone := *connectorErr
	clone.Connector = endpointType
	return &clone
}
