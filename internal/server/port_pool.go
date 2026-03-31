package server

import (
	"errors"
	"sync"
)

// PortPool 端口池管理
type PortPool struct {
	mu       sync.Mutex
	start    int           // 起始端口
	end      int           // 结束端口
	used     map[int]bool  // 已分配端口
	bindings map[int]*Device // 端口 -> 设备映射
	nextPort int           // 下一个尝试分配的端口
}

// NewPortPool 创建端口池
func NewPortPool(start, end int) *PortPool {
	return &PortPool{
		start:    start,
		end:      end,
		used:     make(map[int]bool),
		bindings: make(map[int]*Device),
		nextPort: start,
	}
}

// Allocate 分配一个可用端口
func (p *PortPool) Allocate(device *Device) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 从上次分配的下一个端口开始查找，最多遍历整个范围一次
	for i := 0; i <= p.end-p.start; i++ {
		port := p.nextPort
		p.nextPort++
		if p.nextPort > p.end {
			p.nextPort = p.start
		}
		if !p.used[port] {
			p.used[port] = true
			p.bindings[port] = device
			return port, nil
		}
	}

	return 0, errors.New("port pool exhausted")
}

// Release 释放端口
func (p *PortPool) Release(port int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.used, port)
	delete(p.bindings, port)
}

// GetBinding 获取端口绑定的设备
func (p *PortPool) GetBinding(port int) *Device {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.bindings[port]
}

// GetUsedPorts 获取已使用的端口数量
func (p *PortPool) GetUsedPorts() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.used)
}

// GetAvailablePorts 获取可用端口数量
func (p *PortPool) GetAvailablePorts() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return (p.end - p.start + 1) - len(p.used)
}
