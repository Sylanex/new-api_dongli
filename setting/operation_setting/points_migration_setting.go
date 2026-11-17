package operation_setting

import (
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

// PointsMigrationSetting records the unit boundary applied by the wallet migration.
// A zero cutover leaves the existing quota and payment behavior active.
type PointsMigrationSetting struct {
	CutoverTime        int64 `json:"cutover_time"`
	LegacyQuotaPerUnit int64 `json:"legacy_quota_per_unit"`
}

var pointsMigrationSetting = PointsMigrationSetting{LegacyQuotaPerUnit: 500000}

func init() {
	config.GlobalConfig.Register("points_migration_setting", &pointsMigrationSetting)
}

func GetPointsMigrationSetting() *PointsMigrationSetting {
	return &pointsMigrationSetting
}

func ActivePointsCutoverTime() int64 {
	cutover := pointsMigrationSetting.CutoverTime
	if cutover <= 0 || cutover > time.Now().Unix() {
		return 0
	}
	return cutover
}
