package app

//go:generate go tool mockgen -source=app.go -destination=mock/app_mock.go -package=mock -exclude_interfaces=Compiler
