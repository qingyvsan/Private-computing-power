package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"

	pb "computing-power/proto/v1"
	"computing-power/pkg/wal"
)

// 数据库 Bucket 名称
var (
	bucketNodes     = []byte("nodes")
	bucketJobs      = []byte("jobs")
	bucketUnits     = []byte("units")
	bucketTrust     = []byte("trust_edges")
	bucketInvites   = []byte("invite_codes")
	bucketMeta      = []byte("meta")
	bucketIndexJob  = []byte("index_job_by_owner")
	bucketIndexNode = []byte("index_node_by_status")
	bucketIPAM      = []byte("ipam")
	bucketCRL       = []byte("crl")
)

// Store 是调度器的持久化层（BoltDB 实现）
type Store struct {
	db         *bolt.DB
	walWriter  *wal.Writer
	walEnabled bool
	replaying  bool // WAL 回放模式（不写入 WAL）
}

// Open 打开数据库
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}

	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// init 初始化 Bucket 和 schema 版本
func (s *Store) init() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{
			bucketNodes, bucketJobs, bucketUnits, bucketTrust,
			bucketInvites, bucketMeta, bucketIndexJob, bucketIndexNode,
			bucketIPAM, bucketCRL,
		}
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("create bucket %s: %w", b, err)
			}
		}
		meta := tx.Bucket(bucketMeta)
		if meta.Get([]byte("schema_version")) == nil {
			return meta.Put([]byte("schema_version"), []byte("1"))
		}
		return nil
	})
}

// Close 关闭数据库
func (s *Store) Close() error {
	if s.walWriter != nil {
		s.walWriter.Close()
	}
	return s.db.Close()
}

// EnableWAL 启用 WAL 写入（所有 mutation 操作将同时记录到 WAL）
func (s *Store) EnableWAL(w *wal.Writer) {
	s.walWriter = w
	s.walEnabled = true
}

// GetLastSequence 返回 WAL 最后写入的序列号
func (s *Store) GetLastSequence() uint64 {
	if s.walWriter == nil {
		return 0
	}
	return s.walWriter.LastSequence()
}

// ReadWALFrom 从 WAL 读取从指定序列号开始的条目
func (s *Store) ReadWALFrom(seq uint64) ([]*wal.Entry, error) {
	r, err := wal.NewReader(s.walWriter.Dir())
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.ReadFrom(seq)
}

// walWrite 写入 WAL 条目（仅在 WAL 启用且非回放模式时）
// WAL 写入失败不会阻塞主流程，但会记录错误日志以便排查。
func (s *Store) walWrite(entry wal.Entry) {
	if !s.walEnabled || s.walWriter == nil || s.replaying {
		return
	}
	if _, err := s.walWriter.Write(entry); err != nil {
		log.Printf("wal: write entry (type=%d, key=%s): %v", entry.Type, entry.Key, err)
	}
}

// SetReplaying 设置回放模式（回放时不写入 WAL，避免循环同步）
func (s *Store) SetReplaying(replaying bool) {
	s.replaying = replaying
}

// ========== 节点存储 ==========

// SaveNode 保存节点信息
func (s *Store) SaveNode(n *pb.Node) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		data, err := json.Marshal(n)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(n.ID), data); err != nil {
			return err
		}
		// 更新索引
		idx := tx.Bucket(bucketIndexNode)
		return idx.Put([]byte(fmt.Sprintf("%d:%s", n.Status, n.ID)), []byte(n.ID))
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveNode", Data: mustJSON(n)})
	}
	return err
}

// GetNode 获取节点信息
func (s *Store) GetNode(nodeID string) (*pb.Node, error) {
	var n *pb.Node
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketNodes).Get([]byte(nodeID))
		if data == nil {
			return nil
		}
		var node pb.Node
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		n = &node
		return nil
	})
	return n, err
}

// ListNodes 列出所有节点
func (s *Store) ListNodes() ([]*pb.Node, error) {
	var nodes []*pb.Node
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			var n pb.Node
			if err := json.Unmarshal(v, &n); err != nil {
				return err
			}
			nodes = append(nodes, &n)
			return nil
		})
	})
	return nodes, err
}

// DeleteNode 删除节点
func (s *Store) DeleteNode(nodeID string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketNodes).Delete([]byte(nodeID)); err != nil {
			return err
		}
		return tx.Bucket(bucketIndexNode).ForEach(func(k, v []byte) error {
			if string(v) == nodeID {
				return tx.Bucket(bucketIndexNode).Delete(k)
			}
			return nil
		})
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeDelete, Key: "DeleteNode", Data: []byte(nodeID)})
	}
	return err
}

// ========== 作业存储 ==========

// SaveJob 保存作业
func (s *Store) SaveJob(j *pb.Job) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketJobs)
		data, err := json.Marshal(j)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(j.ID), data); err != nil {
			return err
		}
		// 按 owner 索引
		idx := tx.Bucket(bucketIndexJob)
		return idx.Put([]byte(fmt.Sprintf("%s:%s", j.OwnerID, j.ID)), []byte(j.ID))
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveJob", Data: mustJSON(j)})
	}
	return err
}

// GetJob 获取作业
func (s *Store) GetJob(jobID string) (*pb.Job, error) {
	var j *pb.Job
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketJobs).Get([]byte(jobID))
		if data == nil {
			return nil
		}
		var job pb.Job
		if err := json.Unmarshal(data, &job); err != nil {
			return err
		}
		j = &job
		return nil
	})
	return j, err
}

// ListJobs 列出作业（可按 owner 过滤）
func (s *Store) ListJobs(ownerID string) ([]*pb.Job, error) {
	var jobs []*pb.Job
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketJobs).ForEach(func(k, v []byte) error {
			var j pb.Job
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}
			if ownerID == "" || j.OwnerID == ownerID {
				jobs = append(jobs, &j)
			}
			return nil
		})
	})
	return jobs, err
}

// UpdateJobStatus 更新作业状态
func (s *Store) UpdateJobStatus(jobID string, status pb.JobStatus) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketJobs)
		data := b.Get([]byte(jobID))
		if data == nil {
			return fmt.Errorf("job %s not found", jobID)
		}
		var j pb.Job
		if err := json.Unmarshal(data, &j); err != nil {
			return err
		}
		j.Status = status
		j.UpdatedAt = time.Now().UnixMilli()
		if status == pb.JobStatusCompleted || status == pb.JobStatusFailed || status == pb.JobStatusCancelled {
			j.CompletedAt = time.Now().UnixMilli()
		}
		newData, err := json.Marshal(&j)
		if err != nil {
			return err
		}
		return b.Put([]byte(jobID), newData)
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "UpdateJobStatus", Data: mustJSON(map[string]interface{}{"job_id": jobID, "status": status})})
	}
	return err
}

// ========== Unit 存储 ==========

// SaveUnit 保存 Unit
func (s *Store) SaveUnit(u *pb.Unit) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketUnits)
		data, err := json.Marshal(u)
		if err != nil {
			return err
		}
		return b.Put([]byte(u.ID), data)
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveUnit", Data: mustJSON(u)})
	}
	return err
}

// GetUnit 获取 Unit
func (s *Store) GetUnit(unitID string) (*pb.Unit, error) {
	var u *pb.Unit
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketUnits).Get([]byte(unitID))
		if data == nil {
			return nil
		}
		var unit pb.Unit
		if err := json.Unmarshal(data, &unit); err != nil {
			return err
		}
		u = &unit
		return nil
	})
	return u, err
}

// ListUnitsByStatus 按状态列出 Unit
func (s *Store) ListUnitsByStatus(status pb.UnitStatus) ([]*pb.Unit, error) {
	var units []*pb.Unit
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUnits).ForEach(func(k, v []byte) error {
			var u pb.Unit
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if u.Status == status {
				units = append(units, &u)
			}
			return nil
		})
	})
	return units, err
}

// ListUnitsByJob 列出作业的所有 Unit
func (s *Store) ListUnitsByJob(jobID string) ([]*pb.Unit, error) {
	var units []*pb.Unit
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUnits).ForEach(func(k, v []byte) error {
			var u pb.Unit
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if u.JobID == jobID {
				units = append(units, &u)
			}
			return nil
		})
	})
	return units, err
}

// ListUnitsByStage 按阶段列出 Unit
func (s *Store) ListUnitsByStage(stageID string) ([]*pb.Unit, error) {
	var units []*pb.Unit
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUnits).ForEach(func(k, v []byte) error {
			var u pb.Unit
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if u.StageID == stageID {
				units = append(units, &u)
			}
			return nil
		})
	})
	return units, err
}

// ListUnitsByNodeID 列出指定节点的所有 Unit
func (s *Store) ListUnitsByNodeID(nodeID string) ([]*pb.Unit, error) {
	var units []*pb.Unit
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUnits).ForEach(func(k, v []byte) error {
			var u pb.Unit
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if u.AssignedNode == nodeID {
				units = append(units, &u)
			}
			return nil
		})
	})
	return units, err
}

// UpdateUnitStatus 原子更新 Unit 状态，返回更新后的 Unit
func (s *Store) UpdateUnitStatus(unitID string, status pb.UnitStatus, exitCode int32, errMsg string) (*pb.Unit, error) {
	var u *pb.Unit
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketUnits)
		data := b.Get([]byte(unitID))
		if data == nil {
			return fmt.Errorf("unit %s not found", unitID)
		}
		var unit pb.Unit
		if err := json.Unmarshal(data, &unit); err != nil {
			return err
		}
		unit.Status = status
		unit.ExitCode = exitCode
		unit.ErrorMessage = errMsg
		if status == pb.UnitStatusRunning && unit.StartedAt == 0 {
			unit.StartedAt = time.Now().UnixMilli()
		}
		if status == pb.UnitStatusCompleted || status == pb.UnitStatusFailed || status == pb.UnitStatusCancelled {
			unit.CompletedAt = time.Now().UnixMilli()
		}
		newData, err := json.Marshal(&unit)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(unitID), newData); err != nil {
			return err
		}
		u = &unit
		return nil
	})
	if err == nil && u != nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveUnit", Data: mustJSON(u)})
	}
	return u, err
}

// UpdateStageStatus 更新阶段状态（从 Job 中查找并更新 Stage）
func (s *Store) UpdateStageStatus(stageID string, status pb.StageStatus) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		// 需要遍历 jobs 找到包含该 stage 的 job
		return tx.Bucket(bucketJobs).ForEach(func(k, v []byte) error {
			var j pb.Job
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}
			for _, stage := range j.Stages {
				if stage.ID == stageID {
					stage.Status = status
					j.UpdatedAt = time.Now().UnixMilli()
					newData, err := json.Marshal(&j)
					if err != nil {
						return err
					}
					return tx.Bucket(bucketJobs).Put(k, newData)
				}
			}
			return nil
		})
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "UpdateStageStatus", Data: mustJSON(map[string]interface{}{"stage_id": stageID, "status": status})})
	}
	return err
}

// DeleteJob 级联删除作业及其所有 Unit
func (s *Store) DeleteJob(jobID string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		// 删除关联的 Unit
		unitB := tx.Bucket(bucketUnits)
		if err := unitB.ForEach(func(k, v []byte) error {
			var u pb.Unit
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if u.JobID == jobID {
				return unitB.Delete(k)
			}
			return nil
		}); err != nil {
			return err
		}
		// 删除作业本身
		jobB := tx.Bucket(bucketJobs)
		if err := jobB.Delete([]byte(jobID)); err != nil {
			return err
		}
		// 清理索引
		idx := tx.Bucket(bucketIndexJob)
		return idx.ForEach(func(k, v []byte) error {
			if string(v) == jobID {
				return idx.Delete(k)
			}
			return nil
		})
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeDelete, Key: "DeleteJob", Data: []byte(jobID)})
	}
	return err
}

// ListJobsByStatus 按状态列出作业
func (s *Store) ListJobsByStatus(status pb.JobStatus) ([]*pb.Job, error) {
	var jobs []*pb.Job
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketJobs).ForEach(func(k, v []byte) error {
			var j pb.Job
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}
			if j.Status == status {
				jobs = append(jobs, &j)
			}
			return nil
		})
	})
	return jobs, err
}

// ========== 信任图存储 ==========

// SaveTrustEdge 保存信任边
func (s *Store) SaveTrustEdge(e *pb.TrustEdge) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTrust)
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		return b.Put([]byte(fmt.Sprintf("%s:%s", e.FromNode, e.ToNode)), data)
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveTrustEdge", Data: mustJSON(e)})
	}
	return err
}

// DeleteTrustEdge 删除信任边
func (s *Store) DeleteTrustEdge(from, to string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketTrust).Delete([]byte(fmt.Sprintf("%s:%s", from, to)))
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeDelete, Key: "DeleteTrustEdge", Data: mustJSON(map[string]string{"from": from, "to": to})})
	}
	return err
}

// ListTrustEdges 列出所有信任边
func (s *Store) ListTrustEdges() ([]*pb.TrustEdge, error) {
	var edges []*pb.TrustEdge
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketTrust).ForEach(func(k, v []byte) error {
			var e pb.TrustEdge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			edges = append(edges, &e)
			return nil
		})
	})
	return edges, err
}

// ========== IPAM 存储 ==========

// SaveIPAllocation 保存 IP 分配记录
func (s *Store) SaveIPAllocation(nodeID, ip string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketIPAM).Put([]byte(nodeID), []byte(ip))
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveIPAllocation", Data: mustJSON(map[string]string{"node_id": nodeID, "ip": ip})})
	}
	return err
}

// GetIPAllocation 获取节点的 IP 分配；返回空字符串表示未分配
func (s *Store) GetIPAllocation(nodeID string) (string, error) {
	var ip string
	err := s.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(bucketIPAM).Get([]byte(nodeID))
		if val != nil {
			ip = string(val)
		}
		return nil
	})
	return ip, err
}

// DeleteIPAllocation 删除 IP 分配记录
func (s *Store) DeleteIPAllocation(nodeID string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketIPAM).Delete([]byte(nodeID))
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeDelete, Key: "DeleteIPAllocation", Data: []byte(nodeID)})
	}
	return err
}

// ListIPAllocations 列出所有 IP 分配记录
func (s *Store) ListIPAllocations() (map[string]string, error) {
	allocations := make(map[string]string)
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketIPAM).ForEach(func(k, v []byte) error {
			allocations[string(k)] = string(v)
			return nil
		})
	})
	return allocations, err
}

// ========== 邀请码存储 ==========

// InviteCode 邀请码数据结构
type InviteCode struct {
	Code       string   `json:"code"`
	CreatedBy  string   `json:"created_by"`
	CreatedAt  int64    `json:"created_at"`
	ExpiresAt  int64    `json:"expires_at"`
	MaxUses    int32    `json:"max_uses"`
	UsedCount  int32    `json:"used_count"`
	RedeemedBy []string `json:"redeemed_by,omitempty"`
	Used       bool     `json:"used"`
}

// SaveInviteCode 保存邀请码
func (s *Store) SaveInviteCode(c *InviteCode) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketInvites)
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		return b.Put([]byte(c.Code), data)
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeData, Key: "SaveInviteCode", Data: mustJSON(c)})
	}
	return err
}

// GetInviteCode 获取邀请码，未找到返回 nil
func (s *Store) GetInviteCode(code string) (*InviteCode, error) {
	var ic *InviteCode
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketInvites).Get([]byte(code))
		if data == nil {
			return nil
		}
		var c InviteCode
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		ic = &c
		return nil
	})
	return ic, err
}

// DeleteInviteCode 删除邀请码
func (s *Store) DeleteInviteCode(code string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketInvites).Delete([]byte(code))
	})
	if err == nil {
		s.walWrite(wal.Entry{Type: wal.EntryTypeDelete, Key: "DeleteInviteCode", Data: []byte(code)})
	}
	return err
}

// ListInviteCodes 列出所有邀请码
func (s *Store) ListInviteCodes() ([]*InviteCode, error) {
	var codes []*InviteCode
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketInvites).ForEach(func(k, v []byte) error {
			var c InviteCode
			if err := json.Unmarshal(v, &c); err != nil {
				return err
			}
			codes = append(codes, &c)
			return nil
		})
	})
	return codes, err
}

// GetNodeByFingerprint 根据硬件指纹查找节点，未找到返回 nil
func (s *Store) GetNodeByFingerprint(fingerprint string) (*pb.Node, error) {
	if fingerprint == "" {
		return nil, nil
	}
	var n *pb.Node
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			var node pb.Node
			if err := json.Unmarshal(v, &node); err != nil {
				return err
			}
			if node.HardwareFingerprint == fingerprint {
				n = &node
				return nil // found
			}
			return nil
		})
	})
	return n, err
}

// mustJSON 将对象序列化为 JSON，忽略错误（用于 WAL 写入——WAL 写入失败不应阻塞主流程）
func mustJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(fmt.Sprintf(`{"marshal_error":"%s"}`, err.Error()))
	}
	return data
}

// RevokedCert 记录已吊销的证书
type RevokedCert struct {
	NodeID    string `json:"node_id"`
	Reason    string `json:"reason"`
	RevokedAt int64  `json:"revoked_at"`
}

// SaveRevokedCert 持久化证书吊销记录
func (s *Store) SaveRevokedCert(nodeID, reason string) error {
	rec := RevokedCert{
		NodeID:    nodeID,
		Reason:    reason,
		RevokedAt: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal revoked cert: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketCRL).Put([]byte(nodeID), data)
	})
}

// IsCertRevoked 检查指定节点的证书是否已被吊销
func (s *Store) IsCertRevoked(nodeID string) (bool, error) {
	var revoked bool
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketCRL).Get([]byte(nodeID))
		revoked = data != nil
		return nil
	})
	return revoked, err
}

// ListRevokedCerts 列出所有已吊销的证书记录
func (s *Store) ListRevokedCerts() ([]*RevokedCert, error) {
	var list []*RevokedCert
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketCRL).ForEach(func(k, v []byte) error {
			var rec RevokedCert
			if err := json.Unmarshal(v, &rec); err != nil {
				return nil // skip corrupted entries
			}
			list = append(list, &rec)
			return nil
		})
	})
	return list, err
}

// Backup 创建 BoltDB 一致性快照
func (s *Store) Backup(path string) error {
	return s.db.View(func(tx *bolt.Tx) error {
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create backup: %w", err)
		}
		defer f.Close()
		if _, err := tx.WriteTo(f); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
		return nil
	})
}