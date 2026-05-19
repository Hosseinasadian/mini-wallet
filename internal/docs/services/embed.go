package docs

import _ "embed"

//go:embed docs.html
var Html []byte

//go:embed auth/swagger.json
var AuthSpec []byte

//go:embed wallet/swagger.json
var WalletSpec []byte

//go:embed notification/swagger.json
var NotificationSpec []byte
