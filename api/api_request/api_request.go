package api_request

import (
	"github.com/0xdevelop/NBTerminal/api/api_rpc"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

func ParserRequest(body []byte, r *http.Request) (reqModel *api_rpc.RPCRequest, err error) {
	var tmpMap map[string]interface{}
	err = json.Unmarshal(body, &tmpMap)
	if err != nil {
		return nil, err
	}

	reqModel = &api_rpc.RPCRequest{}

	if method, ok := tmpMap["method"].(string); ok {
		reqModel.Method = method
	} else {
		return nil, errors.New("invalid or missing 'method' field")
	}

	if params, ok := tmpMap["params"].([]interface{}); ok {
		reqModel.Params = params
	} else {
		reqModel.Params = []interface{}{} // 设置为空数组，防止解析错误
	}

	if jsonrpc, ok := tmpMap["jsonrpc"].(string); ok {
		reqModel.JsonRPC = jsonrpc
	} else {
		return nil, errors.New("invalid or missing 'jsonrpc' field")
	}

	if id, ok := tmpMap["id"]; ok {
		reqModel.ID = fmt.Sprintf("%v", id)
	} else {
		reqModel.ID = "" // 设置默认ID为空字符串
	}

	return reqModel, nil
}
