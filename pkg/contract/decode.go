package contract

func DecodeRequestBody(raw []byte) (map[string]any, error) {
	return decodeJSONObject(raw)
}
