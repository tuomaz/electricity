module github.com/tuomaz/electricity

go 1.18

//replace github.com/tuomaz/gohaws => ../gohaws

require (
	github.com/go-co-op/gocron v1.27.0
	github.com/tuomaz/gohaws v0.0.0-20230721152359-a52c82b67d42
	github.com/tuomaz/nordpool v0.0.0-20230426185114-c8b1dd7e3977
)

require (
	github.com/go-resty/resty/v2 v2.7.0 // indirect
	github.com/klauspost/compress v1.16.5 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.0.0-20211029224645-99673261e6eb // indirect
	nhooyr.io/websocket v1.8.7 // indirect
)
