package main

import (
	"context"
	"encoding/json"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const (
	operationLogin           = "login"
	operationRefresh         = "refresh"
	operationLogout          = "logout"
	operationIsAuthenticated = "is_authenticated"
)

type loginParameters struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func dispatchAuthentication(ctx context.Context, client *justetf.Client, operation string, parameters []byte) (json.RawMessage, error) {
	if operation == operationLogin {
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params loginParameters) (json.RawMessage, error) {
			return nil, client.Login(ctx, params.Email, params.Password)
		})
	}
	return dispatchTyped(ctx, parameters, func(ctx context.Context, params struct{}) (json.RawMessage, error) {
		switch operation {
		case operationRefresh:
			return nil, client.Refresh(ctx)
		case operationLogout:
			client.Logout()
			return nil, nil
		case operationIsAuthenticated:
			return json.Marshal(client.IsAuthenticated())
		default:
			return nil, errInvalidArguments
		}
	})
}
