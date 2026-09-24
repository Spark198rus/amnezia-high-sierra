module github.com/spark198rus/amnezia-high-sierra/highsierra-client

go 1.20

require (
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20260814
	golang.org/x/sys v0.30.0
)

require (
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
)

replace github.com/amnezia-vpn/amneziawg-go/v3 => ./third_party/amneziawg-go
