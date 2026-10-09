module e2e_saga

go 1.25.0

replace paylane-jwe => ../../services/paylane-jwe

require paylane-jwe v0.0.0-00010101000000-000000000000

require (
	github.com/go-jose/go-jose/v4 v4.1.5 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
)
