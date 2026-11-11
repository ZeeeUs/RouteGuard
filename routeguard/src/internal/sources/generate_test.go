package sources

//go:generate go tool mockgen -destination=mock/round_tripper_mock.go -package=mock net/http RoundTripper
