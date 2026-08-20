package agent

import (
	"fmt"
	"log"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// UpdateGopsutilMetrics collects TotalMemory, FreeMemory and per-CPU utilization.
func (c *Collector) UpdateGopsutilMetrics() {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("gopsutil: VirtualMemory: %v", err)
	} else {
		c.mu.Lock()
		c.gauge["TotalMemory"] = float64(vmStat.Total)
		c.gauge["FreeMemory"] = float64(vmStat.Free)
		c.mu.Unlock()
	}

	// interval=0 → utilization since last call (non-blocking)
	percs, err := cpu.Percent(0, true)
	if err != nil {
		log.Printf("gopsutil: cpu.Percent: %v", err)
	} else {
		c.mu.Lock()
		for i, p := range percs {
			c.gauge[fmt.Sprintf("CPUutilization%d", i+1)] = p
		}
		c.mu.Unlock()
	}
}
