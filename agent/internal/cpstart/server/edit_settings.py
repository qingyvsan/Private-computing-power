import re

path = "D:/Private-computing-power/agent/internal/cpstart/server/handlers.go"
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

old = '''func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
\t\twriteOK(w, map[string]interface{}{
\t\t\t"agent_name":     h.cfg.Agent.Name,
\t\t\t"scheduler":      h.cfg.Scheduler.Address,
\t\t\t"max_cpu_cores":  h.cfg.Resources.MaxCPUCores,
\t\t\t"max_memory_mb":  h.cfg.Resources.MaxMemoryMB,
\t\t\t"report_gpu":     h.cfg.Resources.ReportGPU,
\t\t\t"node_id":        h.runner.NodeID(),
\t\t\t"agent_status":   h.runner.Status().String(),
\t\t\t"data_dir":       h.cfg.Agent.DataDir,
\t\t})
\t}'''

new = '''func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
\t\twriteOK(w, map[string]interface{}{
\t\t\t"agent_name":       h.cfg.Agent.Name,
\t\t\t"scheduler":        h.cfg.Scheduler.Address,
\t\t\t"max_cpu_cores":    h.cfg.Resources.MaxCPUCores,
\t\t\t"max_memory_mb":    h.cfg.Resources.MaxMemoryMB,
\t\t\t"report_gpu":       h.cfg.Resources.ReportGPU,
\t\t\t"node_id":          h.runner.NodeID(),
\t\t\t"agent_status":     h.runner.Status().String(),
\t\t\t"data_dir":         h.cfg.Agent.DataDir,
\t\t\t"nebula_enabled":   h.cfg.Nebula.Enabled,
\t\t\t"hami_enabled":     h.cfg.HAMI.Enabled,
\t\t\t"updater_enabled":  h.cfg.Updater.Enabled,
\t\t})
\t}'''

if old not in content:
    print("ERROR: old string not found")
    idx = content.find("getSettings")
    if idx >= 0:
        print(repr(content[idx:idx+400]))
else:
    content = content.replace(old, new)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("OK")