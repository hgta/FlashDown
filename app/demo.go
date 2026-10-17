package app

import (
	"fmt"
	"math"
	"time"
)

// EmptyProvider is the null provider: an always-empty grid. main no longer uses
// it - the real engine is behind the same seam now - but it stays as the honest
// empty case for tests and for running the shell with no engine at all.
type EmptyProvider struct{}

func (EmptyProvider) Snapshot() Snapshot {
	return Snapshot{AtMillis: time.Now().UnixMilli()}
}

func (EmptyProvider) Detail(string) TaskDetail { return TaskDetail{} }

// DemoProvider fabricates plausible, moving tasks so the interface can be built
// and judged before the engine exists. Enabled with `flashdown.exe --demo`; it is
// never reachable in a normal run. Progress loops so long UI sessions keep moving.
type DemoProvider struct {
	start time.Time
}

// NewDemoProvider starts the demo clock.
func NewDemoProvider() *DemoProvider {
	return &DemoProvider{start: time.Now()}
}

type demoSpec struct {
	id, name, cat, state string
	size, rate           int64 // rate is the aggregate bytes/sec across all jets
	jets, blocks         int
	resumable            bool
	comment              string
	offset               float64 // frozen progress fraction for paused/error rows
}

// demoSpecs covers every visual state the grid has to render, plus one task that
// does not support resume so that badge gets exercised too.
var demoSpecs = []demoSpec{
	{id: "t1", name: "ubuntu-26.04-desktop-amd64.iso", cat: "software", state: StateDownloading,
		size: 6_100_000_000, rate: 9_400_000, jets: 5, blocks: 48, resumable: true},
	{id: "t2", name: "wow_patch_11.2.7.zip", cat: "game", state: StateDownloading,
		size: 503_316_480, rate: 2_100_000, jets: 5, blocks: 32, resumable: true},
	{id: "t3", name: "GeForce-580.12-driver.cab", cat: "driver", state: StateFinished,
		size: 742_391_808, blocks: 64, resumable: true},
	{id: "t4", name: "podcast_ep12.mp3", cat: "mp3", state: StateDownloading,
		size: 126_877_696, rate: 262_144, jets: 3, blocks: 16, resumable: false,
		comment: "该服务器不支持断点续传"},
	{id: "t5", name: "sdk-tools-win-x64.zip", cat: "software", state: StatePaused,
		size: 1_073_741_824, blocks: 40, resumable: true, offset: 0.41},
	{id: "t6", name: "old_firmware.bin", cat: "driver", state: StateError,
		size: 16_777_216, blocks: 8, resumable: false, offset: 0.07},
}

// Snapshot builds the current demo rows.
func (d *DemoProvider) Snapshot() Snapshot {
	sec := time.Since(d.start).Seconds()

	tasks := make([]TaskDTO, 0, len(demoSpecs))
	var totalSpeed int64
	var downloading, activeJets int

	for i, sp := range demoSpecs {
		t := TaskDTO{
			ID:        sp.id,
			Name:      sp.name,
			URL:       "https://mirror.example.com/releases/" + sp.name,
			Category:  sp.cat,
			State:     sp.state,
			Size:      sp.size,
			Blocks:    sp.blocks,
			Resumable: sp.resumable,
			Comment:   sp.comment,
		}

		switch sp.state {
		case StateFinished:
			t.Done = sp.size
			t.RemainMs = 0

		case StatePaused, StateError:
			t.Done = int64(float64(sp.size) * sp.offset)
			if sp.state == StateError {
				t.Error = "连接被对端重置 (ECONNRESET)"
			}

		default: // downloading
			cycle := float64(sp.size) / float64(sp.rate)
			elapsed := math.Mod(sec, cycle)
			done := int64(float64(sp.rate) * elapsed)
			if done > sp.size {
				done = sp.size
			}

			jitter := 1 + 0.08*math.Sin(sec*1.7+float64(i)*2.1)
			t.Done = done
			t.Speed = int64(float64(sp.rate) * jitter)
			t.ElapsedMs = int64(elapsed * 1000)
			t.RemainMs = int64(float64(sp.size-done) / float64(sp.rate) * 1000)
			t.Jets = sp.jets

			totalSpeed += t.Speed
			downloading++
			activeJets += sp.jets
		}

		tasks = append(tasks, t)
	}

	return Snapshot{
		Tasks:       tasks,
		Downloading: downloading,
		TotalSpeed:  totalSpeed,
		ActiveJets:  activeJets,
		AtMillis:    time.Now().UnixMilli(),
	}
}

// Detail fabricates a block map and jet list for the selected task.
func (d *DemoProvider) Detail(taskID string) TaskDetail {
	for i, sp := range demoSpecs {
		if sp.id != taskID {
			continue
		}

		snap := d.Snapshot()
		var done int64
		var jets int
		for _, t := range snap.Tasks {
			if t.ID == taskID {
				done, jets = t.Done, t.Jets
			}
		}

		return TaskDetail{
			TaskID: taskID,
			Blocks: demoBlocks(sp, done),
			Jets:   demoJets(sp, done, jets),
			Log:    demoLog(sp, i, done),
		}
	}
	return TaskDetail{TaskID: taskID}
}

func demoBlocks(sp demoSpec, done int64) []BlockDTO {
	blocks := make([]BlockDTO, 0, sp.blocks)
	blockSize := sp.size / int64(sp.blocks)
	if blockSize <= 0 {
		blockSize = 1
	}

	for i := 0; i < sp.blocks; i++ {
		start := int64(i) * blockSize
		end := start + blockSize
		if i == sp.blocks-1 {
			end = sp.size
		}

		b := BlockDTO{Start: start, End: end, Jet: -1}
		switch {
		case done >= end:
			b.Done = end - start
		case done > start:
			b.Done = done - start
			b.Jet = i % 5
		}
		blocks = append(blocks, b)
	}
	return blocks
}

func demoJets(sp demoSpec, done int64, jets int) []JetDTO {
	if jets <= 0 {
		return nil
	}
	out := make([]JetDTO, 0, jets)
	remaining := sp.size - done
	if remaining <= 0 {
		return nil
	}

	// Split the remaining range across the active jets, front-loaded so the
	// bars look uneven exactly like a real multi-connection download.
	blockSize := remaining / int64(jets+1)
	if blockSize <= 0 {
		blockSize = 1
	}

	cursor := done
	for i := 0; i < jets; i++ {
		end := cursor + blockSize
		if i == jets-1 {
			end = sp.size
		}
		b := JetDTO{Index: i, Start: cursor, End: end}

		// Give each jet a different amount of progress.
		pct := 0.15 + 0.8*math.Abs(math.Sin(float64(i)*1.9+float64(time.Now().Unix()%7)))
		b.Done = int64(float64(end-cursor) * pct)
		b.Speed = sp.rate / int64(jets)

		out = append(out, b)
		cursor = end
	}
	return out
}

func demoLog(sp demoSpec, idx int, done int64) []string {
	lines := []string{
		fmt.Sprintf("创建任务: %s", sp.name),
		fmt.Sprintf("解析到 %d 个分块", sp.blocks),
	}
	if !sp.resumable {
		lines = append(lines, "服务器未返回 Accept-Ranges，降级为单连接下载")
	}
	lines = append(lines, fmt.Sprintf("已写入 %s", humanBytes(done)))
	if sp.state == StateError {
		lines = append(lines, "第 4 次重试失败: 连接被对端重置 (ECONNRESET)")
	}
	if sp.state == StatePaused {
		lines = append(lines, "用户暂停，已保存断点")
	}
	return lines
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, u := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, u)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}
