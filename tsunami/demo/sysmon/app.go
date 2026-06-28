package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/tsunami/app"
	"github.com/wavetermdev/waveterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{
	Title:     "System Monitor",
	ShortDesc: "Real-time system resource monitor for CPU, memory, and disk",
}

type SystemStats struct {
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsed  float64 `json:"memoryUsed"`
	MemoryTotal float64 `json:"memoryTotal"`
	MemoryPct   float64 `json:"memoryPct"`
	DiskUsed    float64 `json:"diskUsed"`
	DiskTotal   float64 `json:"diskTotal"`
	DiskPct     float64 `json:"diskPct"`
	GoVersion   string  `json:"goVersion"`
	OS          string  `json:"os"`
	Arch        string  `json:"arch"`
	Uptime      string  `json:"uptime"`
	LastUpdated string  `json:"lastUpdated"`
}

type ProgressBarProps struct {
	Label   string  `json:"label"`
	Pct     float64 `json:"pct"`
	Used    float64 `json:"used"`
	Total   float64 `json:"total"`
	Unit    string  `json:"unit"`
	Color   string  `json:"color"`
}

var ProgressBar = app.DefineComponent("ProgressBar", func(props ProgressBarProps) any {
	barWidth := int(props.Pct * 100)
	colorClass := props.Color
	if colorClass == "" {
		if props.Pct > 90 {
			colorClass = "bg-red-500"
		} else if props.Pct > 70 {
			colorClass = "bg-yellow-500"
		} else {
			colorClass = "bg-green-500"
		}
	}

	return vdom.H("div", map[string]any{
		"className": "mb-4",
	},
		vdom.H("div", map[string]any{
			"className": "flex justify-between text-sm mb-1",
		},
			vdom.H("span", nil, props.Label),
			vdom.H("span", map[string]any{
				"className": "text-gray-400",
			}, formatUsage(props.Used, props.Total, props.Unit)),
		),
		vdom.H("div", map[string]any{
			"className": "w-full bg-gray-700 rounded-full h-2.5",
		},
			vdom.H("div", map[string]any{
				"className": colorClass + " h-2.5 rounded-full transition-all duration-500",
				"style": map[string]any{
					"width": barWidth + "%",
				},
			}),
		),
	)
})

var StatCard = app.DefineComponent("StatCard", func(props map[string]any) any {
	label := props["label"].(string)
	value := props["value"].(string)
	icon := props["icon"].(string)

	return vdom.H("div", map[string]any{
		"className": "bg-gray-800 border border-gray-700 rounded-lg p-4",
	},
		vdom.H("div", map[string]any{
			"className": "flex items-center gap-2 mb-2",
		},
			vdom.H("span", map[string]any{
				"className": "text-xl",
			}, icon),
			vdom.H("span", map[string]any{
				"className": "text-gray-400 text-sm",
			}, label),
		),
		vdom.H("div", map[string]any{
			"className": "text-2xl font-bold text-white",
		}, value),
	)
})

var App = app.DefineComponent("App", func(_ any) any {
	statsAtom := app.UseLocal(getDefaultStats())
	refreshAtom := app.UseLocal(true)

	app.UseEffect(func() {
		if !refreshAtom.Get() {
			return
		}
		ticker := time.NewTicker(2 * time.Second)
		statsAtom.Set(getSystemStats())
		return func() {
			ticker.Stop()
		}
	}, []any{refreshAtom.Get()})

	stats := statsAtom.Get()

	return vdom.H("div", map[string]any{
		"className": "max-w-2xl m-5 font-sans text-white",
	},
		vdom.H("div", map[string]any{
			"className": "flex items-center justify-between mb-6",
		},
			vdom.H("h1", map[string]any{
				"className": "text-2xl font-bold flex items-center gap-2",
			},
				vdom.H("span", nil, "📊"),
				vdom.H("span", nil, "System Monitor"),
			),
			vdom.H("button", map[string]any{
				"className": "px-3 py-1.5 text-sm border border-gray-600 rounded hover:bg-gray-700 cursor-pointer",
				"onClick": func() {
					refreshAtom.Set(!refreshAtom.Get())
				},
			}, vdom.If(refreshAtom.Get(), "⏸ Pause").Else("▶ Resume")),
		),

		vdom.H("div", map[string]any{
			"className": "grid grid-cols-2 gap-3 mb-6",
		},
			StatCard(map[string]any{
				"label": "OS",
				"value": stats.OS,
				"icon":  "💻",
			}),
			StatCard(map[string]any{
				"label": "Architecture",
				"value": stats.Arch,
				"icon":  "🔧",
			}),
			StatCard(map[string]any{
				"label": "Go Version",
				"value": stats.GoVersion,
				"icon":  "🐹",
			}),
			StatCard(map[string]any{
				"label": "Uptime",
				"value": stats.Uptime,
				"icon":  "⏱️",
			}),
		),

		vdom.H("div", map[string]any{
			"className": "bg-gray-800 border border-gray-700 rounded-lg p-4 mb-4",
		},
			vdom.H("h2", map[string]any{
				"className": "text-lg font-semibold mb-4 flex items-center gap-2",
			},
				vdom.H("span", nil, "🖥️"),
				vdom.H("span", nil, "CPU"),
			),
			ProgressBar(ProgressBarProps{
				Label:  "CPU Usage",
				Pct:    stats.CPUUsage / 100.0,
				Used:   stats.CPUUsage,
				Total:  100.0,
				Unit:   "%",
				Color:  getCPUColor(stats.CPUUsage),
			}),
		),

		vdom.H("div", map[string]any{
			"className": "bg-gray-800 border border-gray-700 rounded-lg p-4 mb-4",
		},
			vdom.H("h2", map[string]any{
				"className": "text-lg font-semibold mb-4 flex items-center gap-2",
			},
				vdom.H("span", nil, "🧠"),
				vdom.H("span", nil, "Memory"),
			),
			ProgressBar(ProgressBarProps{
				Label:  "RAM",
				Pct:    stats.MemoryPct / 100.0,
				Used:   stats.MemoryUsed,
				Total:  stats.MemoryTotal,
				Unit:   "GB",
				Color:  getMemoryColor(stats.MemoryPct),
			}),
		),

		vdom.H("div", map[string]any{
			"className": "bg-gray-800 border border-gray-700 rounded-lg p-4",
		},
			vdom.H("h2", map[string]any{
				"className": "text-lg font-semibold mb-4 flex items-center gap-2",
			},
				vdom.H("span", nil, "💾"),
				vdom.H("span", nil, "Disk"),
			),
			ProgressBar(ProgressBarProps{
				Label:  "Disk Usage",
				Pct:    stats.DiskPct / 100.0,
				Used:   stats.DiskUsed,
				Total:  stats.DiskTotal,
				Unit:   "GB",
				Color:  getDiskColor(stats.DiskPct),
			}),
		),

		vdom.H("div", map[string]any{
			"className": "text-xs text-gray-500 mt-4 text-center",
		}, "Last updated: "+stats.LastUpdated),
	)
})

func getDefaultStats() SystemStats {
	return SystemStats{
		CPUUsage:    0,
		MemoryUsed:  0,
		MemoryTotal: 0,
		MemoryPct:   0,
		DiskUsed:    0,
		DiskTotal:   0,
		DiskPct:     0,
		GoVersion:   runtime.Version(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Uptime:      "0s",
		LastUpdated: time.Now().Format("15:04:05"),
	}
}

func getCPUColor(cpu float64) string {
	if cpu > 90 {
		return "bg-red-500"
	} else if cpu > 70 {
		return "bg-yellow-500"
	} else if cpu > 50 {
		return "bg-orange-500"
	}
	return "bg-green-500"
}

func getMemoryColor(mem float64) string {
	if mem > 90 {
		return "bg-red-500"
	} else if mem > 70 {
		return "bg-yellow-500"
	} else if mem > 50 {
		return "bg-orange-500"
	}
	return "bg-green-500"
}

func getDiskColor(disk float64) string {
	if disk > 90 {
		return "bg-red-500"
	} else if disk > 70 {
		return "bg-yellow-500"
	} else if disk > 50 {
		return "bg-orange-500"
	}
	return "bg-green-500"
}

func formatUsage(used, total float64, unit string) string {
	if total == 0 {
		return "N/A"
	}
	return formatValue(used, unit) + " / " + formatValue(total, unit)
}

func formatValue(val float64, unit string) string {
	switch unit {
	case "GB":
		s := fmt.Sprintf("%.1f", val)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		return s + "GB"
	case "%":
		s := fmt.Sprintf("%.0f", val)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		return s + "%"
	default:
		return fmt.Sprintf("%.1f %s", val, unit)
	}
}

func getSystemStats() SystemStats {
	stats := getDefaultStats()

	cpu := getCPUUsage()
	memUsed, memTotal := getMemoryUsage()
	diskUsed, diskTotal := getDiskUsage()
	uptime := getUptime()

	stats.CPUUsage = cpu
	stats.MemoryUsed = memUsed
	stats.MemoryTotal = memTotal
	stats.MemoryPct = safePct(memUsed, memTotal)
	stats.DiskUsed = diskUsed
	stats.DiskTotal = diskTotal
	stats.DiskPct = safePct(diskUsed, diskTotal)
	stats.Uptime = uptime
	stats.LastUpdated = time.Now().Format("15:04:05")

	return stats
}

func safePct(used, total float64) float64 {
	if total == 0 {
		return 0
	}
	return (used / total) * 100.0
}

func getCPUUsage() float64 {
	if runtime.GOOS == "windows" {
		return getWindowsCPU()
	}
	return getUnixCPU()
}

func getUnixCPU() float64 {
	cmd := exec.Command("sh", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2}'")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	val := strings.TrimSpace(string(out))
	val = strings.TrimSuffix(val, "%")
	val = strings.TrimSpace(val)
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return 0
	}
	return f
}

func getWindowsCPU() float64 {
	cmd := exec.Command("powershell", "-Command", "(Get-WmiObject Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	val := strings.TrimSpace(string(out))
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return 0
	}
	return f
}

func getMemoryUsage() (usedGB, totalGB float64) {
	if runtime.GOOS == "windows" {
		return getWindowsMemory()
	}
	return getUnixMemory()
}

func getUnixMemory() (usedGB, totalGB float64) {
	cmd := exec.Command("sh", "-c", "free -b | awk '/^Mem:/ {print $2, $3}'")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return 0, 0
	}
	var total, used int64
	if _, err := fmt.Sscanf(parts[0], "%d", &total); err != nil {
		return 0, 0
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &used); err != nil {
		return 0, 0
	}
	return float64(used) / (1024 * 1024 * 1024), float64(total) / (1024 * 1024 * 1024)
}

func getWindowsMemory() (usedGB, totalGB float64) {
	cmd := exec.Command("powershell", "-Command", "$os=Get-WmiObject Win32_OperatingSystem; [math]::Round(($os.TotalVisibleMemorySize - $os.FreePhysicalMemory)/1MB,1); [math]::Round($os.TotalVisibleMemorySize/1MB,1)")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return 0, 0
	}
	var used, total float64
	if _, err := fmt.Sscanf(parts[0], "%f", &used); err != nil {
		return 0, 0
	}
	if _, err := fmt.Sscanf(parts[1], "%f", &total); err != nil {
		return 0, 0
	}
	return used, total
}

func getDiskUsage() (usedGB, totalGB float64) {
	if runtime.GOOS == "windows" {
		return getWindowsDisk()
	}
	return getUnixDisk()
}

func getUnixDisk() (usedGB, totalGB float64) {
	cmd := exec.Command("sh", "-c", "df -BG / | awk 'NR==2 {print $3, $2}'")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return 0, 0
	}
	var used, total float64
	if _, err := fmt.Sscanf(parts[0], "%f", &used); err != nil {
		return 0, 0
	}
	if _, err := fmt.Sscanf(parts[1], "%f", &total); err != nil {
		return 0, 0
	}
	return used, total
}

func getWindowsDisk() (usedGB, totalGB float64) {
	cmd := exec.Command("powershell", "-Command", "$d=Get-WmiObject Win32_LogicalDisk -Filter \"DeviceID='C:'\"; [math]::Round(($d.Size - $d.FreeSpace)/1GB,1); [math]::Round($d.Size/1GB,1)")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return 0, 0
	}
	var used, total float64
	if _, err := fmt.Sscanf(parts[0], "%f", &used); err != nil {
		return 0, 0
	}
	if _, err := fmt.Sscanf(parts[1], "%f", &total); err != nil {
		return 0, 0
	}
	return used, total
}

func getUptime() string {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-Command", "(Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime")
		out, err := cmd.Output()
		if err != nil {
			return "unknown"
		}
		dur, err := time.ParseDuration(strings.TrimSpace(string(out)) + "s")
		if err != nil {
			return "unknown"
		}
		return formatDuration(dur)
	}
	cmd := exec.Command("sh", "-c", "uptime -p")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d days", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d hours", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%d mins", mins))
	}
	if len(parts) == 0 {
		return "just now"
	}
	return strings.Join(parts, " ")
}
