package stats

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/shirou/gopsutil/v3/cpu"
	"golang.org/x/sys/unix"
)

const (
	// cpuInfoTimeout is the timeout used when gathering CPU info. This is used
	// to override the default timeout in gopsutil which has a tendency to
	// timeout on Windows.
	cpuInfoTimeout = 60 * time.Second
)

var (
	cpuMhzPerCore float64
	cpuModelName  string
	cpuNumCores   int
	cpuTotalTicks float64

	initErr error
	onceLer sync.Once
)

func Init() error {
	onceLer.Do(func() {
		var merrs *multierror.Error
		var err error
		if cpuNumCores, err = cpu.Counts(true); err != nil {
			merrs = multierror.Append(merrs, fmt.Errorf("Unable to determine the number of CPU cores available: %v", err))
		}

		var cpuInfo []InfoStat
		ctx, cancel := context.WithTimeout(context.Background(), cpuInfoTimeout)
		defer cancel()
		if cpuInfo, err = InfoWithContext(ctx); err != nil {
			merrs = multierror.Append(merrs, fmt.Errorf("Unable to obtain CPU information: %v", err))
		}

		for _, cpu := range cpuInfo {
			cpuModelName = cpu.ModelName
			cpuMhzPerCore = cpu.Mhz
			break
		}

		// Floor all of the values such that small difference don't cause the
		// node to fall into a unique computed node class
		cpuMhzPerCore = math.Floor(cpuMhzPerCore)
		cpuTotalTicks = math.Floor(float64(cpuNumCores) * cpuMhzPerCore)

		// Set any errors that occurred
		initErr = merrs.ErrorOrNil()
	})
	return initErr
}

// CPUNumCores returns the number of CPU cores available
func CPUNumCores() int {
	return cpuNumCores
}

// CPUMHzPerCore returns the MHz per CPU core
func CPUMHzPerCore() float64 {
	return cpuMhzPerCore
}

// CPUModelName returns the model name of the CPU
func CPUModelName() string {
	return cpuModelName
}

// TotalTicksAvailable calculates the total Mhz available across all cores
func TotalTicksAvailable() float64 {
	return cpuTotalTicks
}

type InfoStat struct {
	CPU        int32    `json:"cpu"`
	VendorID   string   `json:"vendorId"`
	Family     string   `json:"family"`
	Model      string   `json:"model"`
	Stepping   int32    `json:"stepping"`
	PhysicalID string   `json:"physicalId"`
	CoreID     string   `json:"coreId"`
	Cores      int32    `json:"cores"`
	ModelName  string   `json:"modelName"`
	Mhz        float64  `json:"mhz"`
	CacheSize  int32    `json:"cacheSize"`
	Flags      []string `json:"flags"`
	Microcode  string   `json:"microcode"`
}

func InfoWithContext(ctx context.Context) ([]InfoStat, error) {
	var ret []InfoStat

	c := InfoStat{}
	c.ModelName, _ = unix.Sysctl("machdep.cpu.brand_string")
	family, _ := unix.SysctlUint32("machdep.cpu.family")
	c.Family = strconv.FormatUint(uint64(family), 10)
	model, _ := unix.SysctlUint32("machdep.cpu.model")
	c.Model = strconv.FormatUint(uint64(model), 10)
	stepping, _ := unix.SysctlUint32("machdep.cpu.stepping")
	c.Stepping = int32(stepping)
	features, err := unix.Sysctl("machdep.cpu.features")
	if err == nil {
		for _, v := range strings.Fields(features) {
			c.Flags = append(c.Flags, strings.ToLower(v))
		}
	}
	leaf7Features, err := unix.Sysctl("machdep.cpu.leaf7_features")
	if err == nil {
		for _, v := range strings.Fields(leaf7Features) {
			c.Flags = append(c.Flags, strings.ToLower(v))
		}
	}
	extfeatures, err := unix.Sysctl("machdep.cpu.extfeatures")
	if err == nil {
		for _, v := range strings.Fields(extfeatures) {
			c.Flags = append(c.Flags, strings.ToLower(v))
		}
	}
	cores, _ := unix.SysctlUint32("machdep.cpu.core_count")
	c.Cores = int32(cores)
	cacheSize, _ := unix.SysctlUint32("machdep.cpu.cache.size")
	c.CacheSize = int32(cacheSize)
	c.VendorID, _ = unix.Sysctl("machdep.cpu.vendor")

	cpuFrequency := 3200000000
	c.Mhz = float64(cpuFrequency) / 1000000.0

	return append(ret, c), nil
}
