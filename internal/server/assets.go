package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var embeddedWeb embed.FS

func staticHandler() (http.Handler, error) {
	fsContent, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(fsContent)), nil
}
