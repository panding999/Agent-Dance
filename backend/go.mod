module github.com/panding999/agent-dance/backend

go 1.25.0

require (
	modernc.org/sqlite v1.51.0
	nhooyr.io/websocket v1.8.17
)

require (
	code.byted.org/data-speech/wsclientsdk v0.0.0
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.11
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace code.byted.org/data-speech/wsclientsdk => ./internal/doubao/ast/official
