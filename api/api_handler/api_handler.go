package api_handler

import (
	"github.com/0xdevelop/NBTerminal/api/api_config"
	"github.com/0xdevelop/NBTerminal/api/api_request"
	"github.com/0xdevelop/NBTerminal/api/api_response"
	"github.com/0xdevelop/NBTerminal/api/api_rpc"
	"github.com/0xdevelop/NBTerminal/config"
	"errors"
	"fmt"
	"github.com/george012/gtbox"
	"github.com/george012/gtbox/gtbox_log"
	"io"
	"net/http"
	"strings"
)

func apiCommonHandle(r *http.Request) (reqModel *api_rpc.RPCRequest, err error) {

	if r.Method != http.MethodPost {
		return nil, errors.New("Only POST requests are allowed")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	reqModel, err = api_request.ParserRequest(body, r)

	if err != nil {
		return nil, err
	}

	//	检测 请求rpc 方法权限
	AllowedMethod := api_config.CheckAllowedMethods(reqModel.Method)
	if AllowedMethod == false {
		return nil, errors.New("request method is not allowed")
	}
	return reqModel, nil
}

// ApiHandler 处理 HTTP 请求并转发给 TCP 服务器
func ApiHandler(w http.ResponseWriter, r *http.Request) {
	reqModel, err := apiCommonHandle(r)
	if err != nil {
		api_response.HandleResponse(w, err, nil, reqModel)
		return
	}

	api_response.HandleResponse(w, nil, "", reqModel)
}

// HomeHandler 处理根路径请求
func HomeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(""))
}

// Middleware 示例中间件
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		uas := strings.Split(ua, "/")
		if len(uas) > 1 {
			uaName := uas[0]
			if api_config.CheckAllowedUserAgent(uaName) {
				gtbox_log.LogDebugf("Request URI: [%s] ua:[%s]", r.RequestURI, ua)
				next.ServeHTTP(w, r)
				return
			}
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			api_response.HandleResponse(w, err, nil, nil)
			return
		}

		if len(body) == 0 {
			HomeHandler(w, r)
			return
		}

		reqModel, err := api_request.ParserRequest(body, r)
		if err != nil {
			api_response.HandleResponse(w, err, nil, nil)
			return
		}

		// 不符合指定UA，返回失败响应
		err = errors.New(fmt.Sprintf("%s,%s", "permission denied:", r.RemoteAddr))

		if config.CurrentApp.CurrentRunMode == gtbox.RunModeDebug || config.CurrentApp.CurrentRunMode == gtbox.RunModeTest {
			gtbox_log.LogDebugf("permission denied body: %v", body)
			gtbox_log.LogDebugf("permission denied body: %v", reqModel)
		}

		api_response.HandleResponse(w, err, nil, reqModel)
	})
}
