package gotemplates

// APP_VERSION=$(shell git rev-parse --short HEAD)
//
// isuports: test go.mod go.sum *.go cmd/isuports/*
// 	go build -o isuports -ldflags="-X github.com/isucon/isucon12-qualify/webapp/go.AppVersion=$(APP_VERSION)" ./cmd/isuports

var AppVersion string
