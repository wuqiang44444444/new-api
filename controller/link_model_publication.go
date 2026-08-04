package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type rebindLinkModelPublicationRequest struct {
	ContractNamespace string                `json:"contract_namespace"`
	RouteFamily       model.LinkRouteFamily `json:"route_family" binding:"required"`
	CustomerModel     string                `json:"customer_model" binding:"required"`
	LinkSKU           string                `json:"link_sku" binding:"required"`
	ExpectedVersion   int64                 `json:"expected_version" binding:"required"`
	Reason            string                `json:"reason" binding:"required"`
}

type linkModelPublicationView struct {
	model.LinkModelPublication
	CurrentlyFulfillable bool `json:"currently_fulfillable"`
	RoutingConflict      bool `json:"routing_conflict"`
}

func ListLinkModelPublications(c *gin.Context) {
	publications, err := model.ListLinkModelPublications(c.Query("contract_namespace"), model.LinkRouteFamily(c.Query("route_family")), c.Query("customer_model"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	availabilities, err := model.GetLinkModelPublicationAvailabilities(publications, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	views := make([]linkModelPublicationView, 0, len(publications))
	for i := range publications {
		views = append(views, linkModelPublicationView{
			LinkModelPublication: publications[i],
			CurrentlyFulfillable: availabilities[i].CurrentlyFulfillable,
			RoutingConflict:      availabilities[i].RoutingConflict,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": views})
}

func RebindLinkModelPublication(c *gin.Context) {
	var request rebindLinkModelPublicationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid Link publication rebind request"})
		return
	}
	publication, err := model.RebindLinkModelPublication(model.LinkModelPublicationKey{
		ContractNamespace: request.ContractNamespace,
		RouteFamily:       request.RouteFamily,
		CustomerModel:     request.CustomerModel,
	}, request.LinkSKU, request.ExpectedVersion, c.GetInt("id"), request.Reason)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, model.ErrLinkModelPublicationInvalidRebind):
			status = http.StatusBadRequest
		case errors.Is(err, gorm.ErrRecordNotFound):
			status = http.StatusNotFound
		case errors.Is(err, model.ErrLinkModelPublicationVersionConflict),
			errors.Is(err, model.ErrLinkModelPublicationNoopRebind),
			errors.Is(err, model.ErrLinkModelPublicationConcurrentChange),
			errors.Is(err, model.ErrLinkModelPublicationConflict):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": publication})
}
