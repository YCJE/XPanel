package job

import (
	"time"

	"github.com/YCJE/XPanel/database"
	"github.com/YCJE/XPanel/database/model"
	"github.com/YCJE/XPanel/logger"
	"github.com/YCJE/XPanel/web/service"
	"github.com/YCJE/XPanel/xray"
	"gorm.io/gorm"
)

// PeriodicTrafficResetJob resets the traffic counters of every inbound whose
// traffic_reset schedule matches the configured period (hourly/daily/weekly/
// monthly). Client counters are reset alongside the inbound's own totals so
// quota views stay consistent, and lastTrafficResetTime is stamped.
type PeriodicTrafficResetJob struct {
	period         string
	inboundService service.InboundService
}

// NewPeriodicTrafficResetJob creates a job for the given reset period.
func NewPeriodicTrafficResetJob(period string) *PeriodicTrafficResetJob {
	return &PeriodicTrafficResetJob{period: period}
}

// Run performs the reset for all matching inbounds.
func (j *PeriodicTrafficResetJob) Run() {
	inbounds, err := j.inboundService.GetInboundsByTrafficReset(j.period)
	if err != nil {
		logger.Errorf("periodic traffic reset (%s): retrieve inbounds failed: %v", j.period, err)
		return
	}
	if len(inbounds) == 0 {
		return
	}

	now := time.Now().Unix() * 1000
	db := database.GetDB()
	for _, inbound := range inbounds {
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(xray.ClientTraffic{}).
				Where("inbound_id = ?", inbound.Id).
				Updates(map[string]any{"enable": true, "up": 0, "down": 0}).Error; err != nil {
				return err
			}
			return tx.Model(model.Inbound{}).
				Where("id = ?", inbound.Id).
				Updates(map[string]any{"up": 0, "down": 0, "last_traffic_reset_time": now}).Error
		})
		if err != nil {
			logger.Errorf("periodic traffic reset (%s): inbound %d failed: %v", j.period, inbound.Id, err)
			continue
		}
		logger.Infof("periodic traffic reset (%s): inbound %d (%s) reset", j.period, inbound.Id, inbound.Remark)
	}
}
