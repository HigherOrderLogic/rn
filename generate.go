package tui

//go:generate mockgen -destination=./browser/browser_gomock.go -package browser -self_package github.com/ernestrc/go-tui/browser -source ./browser/browser.go
//go:generate mockgen -destination=./proto/gomock_broker.go -package proto -self_package github.com/ernestrc/go-tui/proto -source ./proto/broker.go
//go:generate mockgen -destination=./proto/grpc_gomock.go -package proto google.golang.org/grpc ClientConnInterface
//go:generate mockgen -destination=./plugin/closer_gomock_test.go -package plugin -self_package github.com/ernestrc/go-tui/plugin -source ./plugin/clipboard_rpc_test.go
//go:generate mockgen -destination=./plugin/clipboard_gomock_test.go -package plugin -self_package github.com/ernestrc/go-tui/plugin -source ./plugin/clipboard.go
//go:generate mockgen -destination=./text/event_handler_gomock.go -package text -self_package github.com/ernestrc/go-tui/text -source ./text/event_handler.go
//go:generate mockgen -destination=./text/editor_gomock.go -package text -self_package github.com/ernestrc/go-tui/text -source ./text/editor.go
//go:generate mockgen -destination=./text/event_handler_gomock.go -package text -self_package github.com/ernestrc/go-tui/text -source ./text/event_handler.go
//go:generate mockgen -destination=./workspace/workspace_gomock.go -package workspace -self_package github.com/ernestrc/go-tui/workspace -source ./workspace/workspace.go
//go:generate mockgen -destination=./text/mouse_gomock.go -package text -self_package github.com/ernestrc/go-tui/text -source ./text/mouse.go
