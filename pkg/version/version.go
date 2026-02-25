package version

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"github.com/unrolled/render"
)

var (
	// Version holds the current version of Hanzo Ingress.
	Version = "dev"
	// Codename holds the current version codename of Hanzo Ingress.
	Codename = "cheddar" // beta cheese
	// BuildDate holds the build date of Hanzo Ingress.
	BuildDate = "I don't remember exactly"
	// StartDate holds the start date of Hanzo Ingress.
	StartDate = time.Now()
	// DisableDashboardAd disables ad in the dashboard.
	DisableDashboardAd = false
	// DashboardName holds the custom name for the dashboard.
	DashboardName = ""
)

// Handler expose version routes.
type Handler struct{}

var templatesRenderer = render.New(render.Options{
	Directory: "nowhere",
})

// Append adds version routes on a router.
func (v Handler) Append(router *mux.Router) {
	router.Methods(http.MethodGet).Path("/api/version").
		HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			v := struct {
				Version            string
				Codename           string
				StartDate          time.Time `json:"startDate"`
				UUID               string    `json:"uuid,omitempty"`
				DisableDashboardAd bool      `json:"disableDashboardAd,omitempty"`
				DashboardName      string    `json:"dashboardName,omitempty"`
			}{
				Version:            Version,
				Codename:           Codename,
				StartDate:          StartDate,
				DisableDashboardAd: DisableDashboardAd,
				DashboardName:      DashboardName,
			}

			if err := templatesRenderer.JSON(response, http.StatusOK, v); err != nil {
				log.Error().Err(err).Send()
			}
		})
}

// CheckNewVersion is a no-op for Hanzo Ingress (upstream version check disabled).
func CheckNewVersion() {}
