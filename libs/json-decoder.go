package tshttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Decoder decode request to json structure
func Decoder(w http.ResponseWriter, r *http.Request, v interface{}) error {
	defer r.Body.Close()

	d := json.NewDecoder(r.Body)
	err := d.Decode(v)

	if err == io.EOF {
		//do nothing
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return errors.New("JSON decode error: " + err.Error())
	}
	return nil
}
