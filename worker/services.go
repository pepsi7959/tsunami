package main

// func methodToString(method tsgrpc.Request_HTTPMethod) string {

// 	if method == 0 {
// 		return "GET"
// 	} else if method == 1 {
// 		return "POST"
// 	} else if method == 2 {
// 		return "PUT"
// 	} else if method == 3 {
// 		return "DELETE"
// 	} else if method == 4 {
// 		return "UPDATE"
// 	}

// 	return "Unknown Method"
// }
// func (ctrl *TSControl) Start(req *tsgrpc.Request) (*tsgrpc.Response, error) {

// 	tsConf := tshttp.Conf{
// 		Name:        req.Params.Name,
// 		URL:         req.Params.Url,
// 		Host:        req.Params.Host,
// 		Method:      methodToString(req.Params.Method),
// 		Body:        req.Params.Body,
// 		Concurrence: int(req.Params.MaxConcurrences),
// 	}

// 	if ctrl.services[tsConf.Name] == nil {
// 		go StartApp(tsConf.Name, ctrl, tsConf)
// 	}

// 	data := struct {
// 		url  string
// 		name string
// 	}{
// 		url:  "http://" + tshttp.GetIP().String() + ":8091" + APIVersion,
// 		name: tsConf.Name,
// 	}
// 	jdata, _ := json.Marshal(data)

// 	return &tsgrpc.Response{
// 		ErrorCode: 200,
// 		Data:      string(jdata),
// 	}, nil

// }
