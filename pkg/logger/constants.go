package logger

type Layer string

const (
	LayerMain       Layer = "main"
	LayerHTTP       Layer = "http"
	LayerService    Layer = "service"
	LayerRepository Layer = "repository"
	LayerMysql      Layer = "mysql"
)

type App string

const (
	AppAuth         App = "auth"
	AppWallet       App = "wallet"
	AppDocs         App = "docs"
	AppNotification App = "notification"
)
