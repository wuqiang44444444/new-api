package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const customerExportArchiveCookie = "customer_export_archive"

func CreateSelfCustomerExportShareArchive(c *gin.Context) {
	source, ok := loadOwnedCustomerExport(c)
	if !ok {
		return
	}
	job, err := service.SubmitCustomerExportShareArchive(c.Request.Context(), source)
	if err != nil {
		if errors.Is(err, service.ErrCustomerExportArchiveUnavailable) {
			c.Status(http.StatusGone)
			return
		}
		respondCustomerExportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": customerExportView(c, job)})
}

func GetSelfCustomerExportShareLink(c *gin.Context) {
	job, ok := loadOwnedCustomerExport(c)
	if !ok {
		return
	}
	url, expiresAt, err := service.PresignCustomerExportShareArchive(c.Request.Context(), job)
	if err != nil {
		if errors.Is(err, service.ErrCustomerExportArchiveUnavailable) {
			c.Status(http.StatusGone)
			return
		}
		respondCustomerExportError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"url": url, "expires_at": expiresAt})
}

func PrepareSelfCustomerExportArchive(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	if _, ok := loadCustomerExportArchive(c, identity.UserID); !ok {
		return
	}
	ticket, err := service.IssueCustomerExportArchiveTicket(identity, c.Param("job_id"))
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	// Scope the proof to this archive path. Native attachment requests do
	// not carry the dashboard's Authorization header.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     customerExportArchiveCookie,
		Value:    ticket,
		Path:     c.Request.URL.Path,
		MaxAge:   60,
		HttpOnly: true,
		Secure:   common.SessionCookieSecure || c.Request.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	c.Status(http.StatusNoContent)
}

func DownloadSelfCustomerExportArchive(c *gin.Context) {
	cookie, err := c.Request.Cookie(customerExportArchiveCookie)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	userID, err := service.ValidateCustomerExportArchiveTicket(cookie.Value, c.Param("job_id"))
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	archive, ok := loadCustomerExportArchive(c, userID)
	if !ok {
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="customer-export.zip"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Accel-Buffering", "no")
	if err := archive.Write(c.Request.Context(), c.Writer); err != nil {
		if c.Writer.Written() {
			if c.Request.Context().Err() == nil {
				logger.LogWarn(c.Request.Context(), "customer export archive interrupted")
			}
			return
		}
		c.Header("Content-Disposition", "")
		c.Header("Content-Type", "")
		if errors.Is(err, service.ErrCustomerExportArchiveBusy) {
			c.Status(http.StatusTooManyRequests)
			return
		}
		c.Status(http.StatusServiceUnavailable)
	}
}

func loadCustomerExportArchive(c *gin.Context, userID int) (*service.CustomerExportArchive, bool) {
	job, err := model.GetCustomerExportJobForOwner(c.Param("job_id"), userID)
	if err != nil {
		respondCustomerExportError(c, err)
		return nil, false
	}
	if job.Status != model.CustomerExportJobStatusSucceeded || job.ExpiresAt <= common.GetTimestamp() {
		c.Status(http.StatusGone)
		return nil, false
	}
	archive, err := service.PrepareCustomerExportArchive(job)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return nil, false
	}
	return archive, true
}
