package graph

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

import "github.com/wricardo/ai-http-bin/internal/store"

type Resolver struct {
	Store   *store.Store
	BaseURL string
}
