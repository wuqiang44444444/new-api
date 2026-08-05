package main

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	payload, err := common.Marshal(model.PublicModelArkVideoCapabilityProjection())
	if err != nil {
		panic(err)
	}
	fmt.Println(string(payload))
}
