// Package repo 实现数据源管理，包括下载、解析和存储 IP 段数据
package repo

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ApnicURL APNIC delegated 数据默认下载地址
	ApnicURL = "http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest"
	// DownloadTimeout 下载超时时间
	DownloadTimeout = 120 * time.Second
)

// IPData 解析后的 IP 段数据
type IPData struct {
	IPv4 []string // IPv4 CIDR 列表
	IPv6 []string // IPv6 CIDR 列表
}

// Download 从 URL 下载数据并返回内容
func Download(url string) ([]byte, error) {
	client := &http.Client{
		Timeout: DownloadTimeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应数据失败: %w", err)
	}

	return data, nil
}

// ParseCIDRFile 解析 CIDR 格式的 IP 段文件（每行一个 CIDR）
func ParseCIDRFile(data []byte, ipVersion string) (*IPData, error) {
	result := &IPData{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// 验证 CIDR 格式
		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			// 尝试作为单个 IP 处理
			ip := net.ParseIP(line)
			if ip == nil {
				continue // 跳过无效行
			}
			if ip.To4() != nil {
				line = line + "/32"
			} else {
				line = line + "/128"
			}
			_, ipNet, _ = net.ParseCIDR(line)
		}

		cidr := ipNet.String()

		if ipNet.IP.To4() != nil {
			if ipVersion == "" || ipVersion == "ipv4" {
				result.IPv4 = append(result.IPv4, cidr)
			}
		} else {
			if ipVersion == "" || ipVersion == "ipv6" {
				result.IPv6 = append(result.IPv6, cidr)
			}
		}
	}

	return result, nil
}

// ParseAPNIC 解析 APNIC delegated 格式数据
// countryCode 为国家代码（如 cn, jp, us）
func ParseAPNIC(data []byte, countryCode string) (*IPData, error) {
	result := &IPData{}
	countryCode = strings.ToUpper(countryCode)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过注释和头部
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// APNIC 格式: registry|cc|type|start|value|date|status[|extensions]
		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			continue
		}

		// 跳过头部行
		if parts[0] == "2" || parts[1] == "*" {
			continue
		}

		cc := strings.ToUpper(parts[1])
		ipType := strings.ToLower(parts[2])
		start := parts[3]
		value := parts[4]

		if cc != countryCode {
			continue
		}

		switch ipType {
		case "ipv4":
			cidr := ipv4ToCIDR(start, value)
			if cidr != "" {
				result.IPv4 = append(result.IPv4, cidr)
			}
		case "ipv6":
			cidr := fmt.Sprintf("%s/%s", start, value)
			// 验证 IPv6 CIDR
			_, _, err := net.ParseCIDR(cidr)
			if err == nil {
				result.IPv6 = append(result.IPv6, cidr)
			}
		}
	}

	return result, nil
}

// ipv4ToCIDR 将 APNIC 的 IPv4 起始地址+数量转换为 CIDR
// APNIC 格式中 value 是 IP 数量（总是 2 的幂次）
func ipv4ToCIDR(start, countStr string) string {
	ip := net.ParseIP(start)
	if ip == nil {
		return ""
	}

	var count float64
	fmt.Sscanf(countStr, "%f", &count)
	if count <= 0 {
		return ""
	}

	// 计算前缀长度: prefix = 32 - log2(count)
	prefix := 32 - int(math.Log2(count))
	if prefix < 0 || prefix > 32 {
		return ""
	}

	return fmt.Sprintf("%s/%d", start, prefix)
}

// ReadLocalFile 读取本地文件
func ReadLocalFile(path string) ([]byte, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析路径失败: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	return data, nil
}

// SaveIPData 将 IP 段数据保存到文件
func SaveIPData(dataPath string, ipData *IPData) error {
	dir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	var lines []string
	for _, cidr := range ipData.IPv4 {
		lines = append(lines, cidr)
	}
	for _, cidr := range ipData.IPv6 {
		lines = append(lines, cidr)
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(dataPath, []byte(content), 0644)
}

// LoadIPData 从文件加载 IP 段数据
func LoadIPData(dataPath string) (*IPData, error) {
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, fmt.Errorf("读取数据文件失败: %w", err)
	}

	return ParseCIDRFile(data, "")
}

// FetchAndParse 下载并解析数据源
func FetchAndParse(repoType, source string) (*IPData, error) {
	var data []byte
	var err error

	// 判断是否为 APNIC 类型
	if strings.HasPrefix(repoType, "apnic:") {
		countryCode := strings.TrimPrefix(repoType, "apnic:")
		if countryCode == "" {
			return nil, fmt.Errorf("APNIC 类型需要指定国家代码，如 apnic:cn")
		}

		url := source
		if url == "" {
			url = ApnicURL
		}

		data, err = Download(url)
		if err != nil {
			return nil, err
		}

		return ParseAPNIC(data, countryCode)
	}

	// IPv4 或 IPv6 类型
	if source == "" {
		return nil, fmt.Errorf("需要指定数据源 URL 或文件路径")
	}

	// 判断是 URL 还是本地文件
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		data, err = Download(source)
	} else {
		data, err = ReadLocalFile(source)
	}
	if err != nil {
		return nil, err
	}

	return ParseCIDRFile(data, repoType)
}
