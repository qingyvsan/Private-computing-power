package trustgraph

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrustEdge 表示一条信任声明
type TrustEdge struct {
	From      string     `json:"from"`
	To        string     `json:"to"`
	Signature []byte     `json:"signature"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Graph 是信任图的内存表示，由调度器维护
// 信任关系是单向的：A 信任 B 不代表 B 信任 A
type Graph struct {
	mu      sync.RWMutex
	edges   map[string]map[string]*TrustEdge // from -> to -> edge
	reverse map[string]map[string]*TrustEdge // to -> from -> edge (反向索引)
}

// NewGraph 创建信任图
func NewGraph() *Graph {
	return &Graph{
		edges:   make(map[string]map[string]*TrustEdge),
		reverse: make(map[string]map[string]*TrustEdge),
	}
}

// AddEdge 添加信任声明
func (g *Graph) AddEdge(from, to string, sig []byte, expiresAt *time.Time) error {
	if from == to {
		return fmt.Errorf("self-trust is not allowed")
	}

	edge := &TrustEdge{
		From:      from,
		To:        to,
		Signature: sig,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.edges[from] == nil {
		g.edges[from] = make(map[string]*TrustEdge)
	}
	g.edges[from][to] = edge

	if g.reverse[to] == nil {
		g.reverse[to] = make(map[string]*TrustEdge)
	}
	g.reverse[to][from] = edge

	return nil
}

// RemoveEdge 移除信任声明
func (g *Graph) RemoveEdge(from, to string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.edges[from]; ok {
		delete(g.edges[from], to)
		if len(g.edges[from]) == 0 {
			delete(g.edges, from)
		}
	}
	if _, ok := g.reverse[to]; ok {
		delete(g.reverse[to], from)
		if len(g.reverse[to]) == 0 {
			delete(g.reverse, to)
		}
	}
	return nil
}

// HasTrust 检查 from 是否信任 to
func (g *Graph) HasTrust(from, to string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges, ok := g.edges[from]
	if !ok {
		return false
	}
	edge, ok := edges[to]
	if !ok {
		return false
	}
	// 检查是否过期
	if edge.ExpiresAt != nil && time.Now().After(*edge.ExpiresAt) {
		return false
	}
	return true
}

// GetTrustedNodes 返回 node 信任的节点列表（出边）
func (g *Graph) GetTrustedNodes(node string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges, ok := g.edges[node]
	if !ok {
		return nil
	}
	now := time.Now()
	var result []string
	for to, edge := range edges {
		if edge.ExpiresAt != nil && now.After(*edge.ExpiresAt) {
			continue
		}
		result = append(result, to)
	}
	return result
}

// GetTrustedBy 返回信任 node 的节点列表（入边）
func (g *Graph) GetTrustedBy(node string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges, ok := g.reverse[node]
	if !ok {
		return nil
	}
	now := time.Now()
	var result []string
	for from, edge := range edges {
		if edge.ExpiresAt != nil && now.After(*edge.ExpiresAt) {
			continue
		}
		result = append(result, from)
	}
	return result
}

// IsReachable 检查是否存在信任路径 from -> ... -> to（BFS，深度限制）
func (g *Graph) IsReachable(from, to string, maxDepth int) bool {
	if from == to {
		return true
	}
	if maxDepth <= 0 {
		maxDepth = 10
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	queue := []string{from}
	visited[from] = true
	depth := 0

	for len(queue) > 0 && depth < maxDepth {
		next := queue[0]
		queue = queue[1:]

		edges, ok := g.edges[next]
		if !ok {
			continue
		}

		now := time.Now()
		for target, edge := range edges {
			if edge.ExpiresAt != nil && now.After(*edge.ExpiresAt) {
				continue
			}
			if target == to {
				return true
			}
			if !visited[target] {
				visited[target] = true
				queue = append(queue, target)
			}
		}
		depth++
	}
	return false
}

// GetAllEdges 返回所有信任边
func (g *Graph) GetAllEdges() []*TrustEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	now := time.Now()
	var result []*TrustEdge
	for _, edges := range g.edges {
		for _, edge := range edges {
			if edge.ExpiresAt != nil && now.After(*edge.ExpiresAt) {
				continue
			}
			result = append(result, edge)
		}
	}
	return result
}

// PruneExpired 清理过期边，返回清理数量
func (g *Graph) PruneExpired() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	count := 0

	for from, edges := range g.edges {
		for to, edge := range edges {
			if edge.ExpiresAt != nil && now.After(*edge.ExpiresAt) {
				delete(g.edges[from], to)
				delete(g.reverse[to], from)
				count++
			}
		}
		if len(g.edges[from]) == 0 {
			delete(g.edges, from)
		}
	}
	return count
}

// Size 返回信任图大小（边数）
func (g *Graph) Size() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	count := 0
	for _, edges := range g.edges {
		count += len(edges)
	}
	return count
}

// ========== 签名工具 ==========

// SignTrust 使用 ECDSA 私钥签署信任声明
func SignTrust(key *ecdsa.PrivateKey, from, to string) ([]byte, error) {
	msg := fmt.Sprintf("trust:%s:%s", from, to)
	hash := sha256.Sum256([]byte(msg))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return nil, fmt.Errorf("sign failed: %w", err)
	}
	// 固定 64 字节编码（各 32 字节），确保 VerifyTrust 可正确分割
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return sig, nil
}

// VerifyTrust 验证信任声明的 ECDSA 签名
func VerifyTrust(pubKeyBytes []byte, from, to string, sig []byte) error {
	block, _ := pem.Decode(pubKeyBytes)
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}
	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}
	pubKey, ok := pubInterface.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("not an ECDSA public key")
	}

	msg := fmt.Sprintf("trust:%s:%s", from, to)
	hash := sha256.Sum256([]byte(msg))

	r := new(big.Int)
	s := new(big.Int)
	half := len(sig) / 2
	r.SetBytes(sig[:half])
	s.SetBytes(sig[half:])

	if !ecdsa.Verify(pubKey, hash[:], r, s) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// GenerateKey 生成 ECDSA P-256 密钥对
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// MarshalPublicKey 将公钥编码为 PEM 格式
func MarshalPublicKey(key *ecdsa.PublicKey) ([]byte, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}), nil
}

// SavePrivateKey 保存 ECDSA 私钥到 PEM 文件
func SavePrivateKey(key *ecdsa.PrivateKey, path string) error {
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	pemBlock := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}
	return os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0600)
}

// LoadPrivateKey 从 PEM 文件加载 ECDSA 私钥
func LoadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

// LoadOrGenerateKey 加载已有密钥或生成新密钥
func LoadOrGenerateKey(path string) (*ecdsa.PrivateKey, error) {
	key, err := LoadPrivateKey(path)
	if err == nil {
		return key, nil
	}
	key, err = GenerateKey()
	if err != nil {
		return nil, err
	}
	if err := SavePrivateKey(key, path); err != nil {
		return nil, err
	}
	return key, nil
}