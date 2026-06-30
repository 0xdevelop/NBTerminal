package api

import (
	"github.com/0xdevelop/NBTerminal/api/api_config"
	"github.com/0xdevelop/NBTerminal/api/api_handler"
	"fmt"
	"github.com/george012/gtbox/gtbox_log"
	"github.com/gorilla/mux"
	"net/http"
)

func StartAPIService(apiCfg *api_config.ApiConfig) {

	api_config.CurrentApiConfig = apiCfg

	go func() {
		muxRouter := mux.NewRouter()
		muxRouter.Use(api_handler.Middleware) // 使用中间件
		muxRouter.HandleFunc("/", api_handler.HomeHandler).Methods("GET")
		muxRouter.HandleFunc("/api/v1", api_handler.ApiHandler).Methods("POST")

		addr := fmt.Sprintf("%s:%d", "0.0.0.0", apiCfg.Port)
		gtbox_log.LogInfof("API server Run On  [%s]", fmt.Sprintf("http://127.0.0.1:%d", apiCfg.Port))
		if err := http.ListenAndServe(addr, muxRouter); err != nil {
			gtbox_log.LogErrorf("Failed to start HTTP server: %v\n", err)
		}
	}()

}
