package api_response

import (
	"github.com/0xdevelop/NBTerminal/api/api_rpc"
	"encoding/json"
	"net/http"
)

func HandleResponse(w http.ResponseWriter, err error, respData interface{}, reqModel *api_rpc.RPCRequest) {
	w.Header().Set("Content-Type", "application/json")

	aID := "1"
	if reqModel != nil {
		aID = reqModel.ID
	}

	resp := &api_rpc.RPCResponse{
		JsonRPC: "2.0",
		ID:      aID,
	}

	if err != nil {
		errMap := map[string]interface{}{
			"error_code": "-1",
			"error_msg":  err.Error(),
		}
		resp.Error = errMap
	} else {
		resp.Result = respData
	}

	jResp, _ := json.Marshal(resp)
	w.Write(append(jResp, '\n'))
}
