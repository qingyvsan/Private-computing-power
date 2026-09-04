package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketNodes     = []byte("nodes")
	bucketJobs      = []byte("jobs")
	bucketUnits     = []byte("units")
	bucketTrust     = []byte("trust_edges")
	bucketInvites   = []byte("invite_codes")
	bucketIndexJob  = []byte("index_job_by_owner")
	bucketIndexNode = []byte("index_node_by_status")
	bucketIPAM      = []byte("ipam")
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <scheduler.db> [node_id...]", os.Args[0])
	}

	dbPath := os.Args[1]
	nodeIDs := os.Args[2:]
	if len(nodeIDs) == 0 {
		nodeIDs = []string{"node-53089-qingyvsa"}
	}

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 先列出所有节点
	fmt.Println("=== Current nodes ===")
	db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			var node map[string]interface{}
			json.Unmarshal(v, &node)
			name, _ := node["name"].(string)
			id := string(k)
			fp, _ := node["hardwareFingerprint"].(string)
			fmt.Printf("  %s (name=%s, fingerprint=%s)\n", id, name, fp)
			return nil
		})
	})

	fmt.Println("\n=== Current jobs ===")
	db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketJobs).ForEach(func(k, v []byte) error {
			var job map[string]interface{}
			json.Unmarshal(v, &job)
			name, _ := job["name"].(string)
			status, _ := job["status"].(string)
			fmt.Printf("  %s (name=%s, status=%s)\n", string(k), name, status)
			return nil
		})
	})

	fmt.Println("\n=== Current units ===")
	db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketUnits).ForEach(func(k, v []byte) error {
			var unit map[string]interface{}
			json.Unmarshal(v, &unit)
			jobID, _ := unit["jobId"].(string)
			nodeID, _ := unit["nodeId"].(string)
			status, _ := unit["status"].(string)
			fmt.Printf("  %s (job=%s, node=%s, status=%s)\n", string(k), jobID, nodeID, status)
			return nil
		})
	})

	// 删除指定节点
	for _, nodeID := range nodeIDs {
		fmt.Printf("\n=== Deleting node %s ===\n", nodeID)
		err := db.Update(func(tx *bolt.Tx) error {
			// 删除节点记录
			if err := tx.Bucket(bucketNodes).Delete([]byte(nodeID)); err != nil {
				return fmt.Errorf("delete node %s: %w", nodeID, err)
			}
			// 删除节点状态索引
			idx := tx.Bucket(bucketIndexNode)
			toDelete := [][]byte{}
			idx.ForEach(func(k, v []byte) error {
				if string(v) == nodeID {
					toDelete = append(toDelete, append([]byte{}, k...))
				}
				return nil
			})
			for _, k := range toDelete {
				idx.Delete(k)
			}
			fmt.Printf("  deleted node %s\n", nodeID)
			return nil
		})
		if err != nil {
			log.Printf("  error: %v", err)
		}

		// 删除 IPAM 分配
		db.Update(func(tx *bolt.Tx) error {
			ipam := tx.Bucket(bucketIPAM)
			if ipam == nil {
				return nil
			}
			toDelete := [][]byte{}
			ipam.ForEach(func(k, v []byte) error {
				if string(v) == nodeID {
					toDelete = append(toDelete, append([]byte{}, k...))
				}
				return nil
			})
			for _, k := range toDelete {
				ipam.Delete(k)
				fmt.Printf("  deleted ipam key %s\n", string(k))
			}
			return nil
		})
	}

	// 删除所有作业和 unit
	fmt.Println("\n=== Deleting all jobs and units ===")
	db.Update(func(tx *bolt.Tx) error {
		// 清空作业
		jobs := tx.Bucket(bucketJobs)
		delKeys := [][]byte{}
		jobs.ForEach(func(k, v []byte) error {
			delKeys = append(delKeys, append([]byte{}, k...))
			return nil
		})
		for _, k := range delKeys {
			jobs.Delete(k)
		}
		fmt.Printf("  deleted %d jobs\n", len(delKeys))

		// 清空 unit
		units := tx.Bucket(bucketUnits)
		delKeys = [][]byte{}
		units.ForEach(func(k, v []byte) error {
			delKeys = append(delKeys, append([]byte{}, k...))
			return nil
		})
		for _, k := range delKeys {
			units.Delete(k)
		}
		fmt.Printf("  deleted %d units\n", len(delKeys))

		// 清空作业索引
		jobIdx := tx.Bucket(bucketIndexJob)
		delKeys = [][]byte{}
		jobIdx.ForEach(func(k, v []byte) error {
			delKeys = append(delKeys, append([]byte{}, k...))
			return nil
		})
		for _, k := range delKeys {
			jobIdx.Delete(k)
		}
		fmt.Printf("  deleted %d job index entries\n", len(delKeys))

		return nil
	})

	fmt.Println("\n=== Verification ===")
	db.View(func(tx *bolt.Tx) error {
		count := 0
		tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			count++
			return nil
		})
		fmt.Printf("  remaining nodes: %d\n", count)

		count = 0
		tx.Bucket(bucketJobs).ForEach(func(k, v []byte) error {
			count++
			return nil
		})
		fmt.Printf("  remaining jobs: %d\n", count)

		count = 0
		tx.Bucket(bucketUnits).ForEach(func(k, v []byte) error {
			count++
			return nil
		})
		fmt.Printf("  remaining units: %d\n", count)
		return nil
	})

	fmt.Println("\nDone.")
}