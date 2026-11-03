package accountdeletion

import (
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Listen serves the form and the /v1 endpoints. addr "off" or empty skips it.
// A bind failure is logged and does not stop the process that called Listen.
func Listen(addr string, deps Deps) {
	if addr == "" || addr == "off" {
		return
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           New(deps).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("account deletion listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("account deletion http: %v", err)
		}
	}()
}
