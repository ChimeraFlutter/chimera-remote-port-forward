package main

/*
#include <stdlib.h>
#include <string.h>

typedef void (*StateCallback)(const char* deviceName, int state, int remotePort, const char* message);

static StateCallback g_callback = NULL;

static void SetCallbackC(StateCallback cb) {
    g_callback = cb;
}

static void CallCallbackC(const char* deviceName, int state, int remotePort, const char* message) {
    if (g_callback != NULL) {
        g_callback(deviceName, state, remotePort, message);
    }
}
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/chimera/chimera-remote-port-forward/internal/client"
	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
)

var (
	multiClient *client.MultiClient
	mcMu        sync.RWMutex
	log         *logger.Logger
	loggerOnce  sync.Once
)

// 状态常量，对应 Flutter 端
const (
	StateDisconnected = 0
	StateConnecting   = 1
	StateConnected    = 2
	StateError        = 3
)

//export Initialize
func Initialize(server *C.char, token *C.char) C.int {
	mcMu.Lock()
	defer mcMu.Unlock()

	if multiClient != nil {
		return 0 // 已初始化
	}

	goServer := C.GoString(server)
	goToken := C.GoString(token)

	// 初始化日志
	loggerOnce.Do(func() {
		var err error
		log, err = logger.NewLogger(&logger.Config{
			BaseDir:     "",
			ServiceName: "dll",
			MaxAge:      7 * 24 * time.Hour,
		})
		if err != nil {
			fmt.Printf("Failed to create logger: %v\n", err)
		}
	})

	multiClient = client.NewMultiClient(goServer, goToken, stateCallback)
	if log != nil {
		multiClient.SetLogger(log)
	}

	return 0
}

//export SetStateCallback
func SetStateCallback(cb C.StateCallback) C.int {
	C.SetCallbackC(cb)
	return 0
}

// stateCallback 内部状态回调
func stateCallback(deviceName string, state int, remotePort int, message string) {
	cDeviceName := C.CString(deviceName)
	cMessage := C.CString(message)
	defer C.free(unsafe.Pointer(cDeviceName))
	defer C.free(unsafe.Pointer(cMessage))

	C.CallCallbackC(cDeviceName, C.int(state), C.int(remotePort), cMessage)
}

//export AddPort
func AddPort(deviceName *C.char, localIP *C.char, localPort C.int) C.int {
	mcMu.RLock()
	defer mcMu.RUnlock()

	if multiClient == nil {
		return -1
	}

	goDeviceName := C.GoString(deviceName)
	goLocalIP := C.GoString(localIP)
	err := multiClient.AddPort(goDeviceName, goLocalIP, int(localPort))
	if err != nil {
		if log != nil {
			log.Error("AddPort failed", logger.String("error", err.Error()))
		}
		stateCallback(goDeviceName, StateError, 0, err.Error())
		return -1
	}

	return 0
}

//export RemovePort
func RemovePort(deviceName *C.char) C.int {
	mcMu.RLock()
	defer mcMu.RUnlock()

	if multiClient == nil {
		return -1
	}

	goDeviceName := C.GoString(deviceName)
	err := multiClient.RemovePort(goDeviceName)
	if err != nil {
		return -1
	}

	return 0
}

//export GetPortCount
func GetPortCount() C.int {
	mcMu.RLock()
	defer mcMu.RUnlock()

	if multiClient == nil {
		return 0
	}
	return C.int(len(multiClient.GetPorts()))
}

//export GetPortInfo
func GetPortInfo(index C.int, deviceNameBuf *C.char, bufSize C.int, localIPBuf *C.char, localIPBufSize C.int, localPort *C.int, remotePort *C.int) C.int {
	mcMu.RLock()
	defer mcMu.RUnlock()

	if multiClient == nil {
		return -1
	}

	ports := multiClient.GetPorts()
	idx := int(index)
	if idx < 0 || idx >= len(ports) {
		return -1
	}

	info := ports[idx]

	// 复制设备名到缓冲区
	goName := []byte(info.DeviceName)
	copyLen := len(goName)
	if copyLen >= int(bufSize) {
		copyLen = int(bufSize) - 1
	}
	C.memcpy(unsafe.Pointer(deviceNameBuf), unsafe.Pointer(&goName[0]), C.size_t(copyLen))
	*(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(deviceNameBuf)) + uintptr(copyLen))) = 0

	// 复制 localIP 到缓冲区
	goLocalIP := []byte(info.LocalIP)
	copyLen = len(goLocalIP)
	if copyLen >= int(localIPBufSize) {
		copyLen = int(localIPBufSize) - 1
	}
	C.memcpy(unsafe.Pointer(localIPBuf), unsafe.Pointer(&goLocalIP[0]), C.size_t(copyLen))
	*(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(localIPBuf)) + uintptr(copyLen))) = 0

	*localPort = C.int(info.LocalPort)
	*remotePort = C.int(info.RemotePort)

	return 0
}

//export Stop
func Stop() C.int {
	mcMu.Lock()
	defer mcMu.Unlock()

	if multiClient != nil {
		multiClient.Stop()
		multiClient = nil
	}
	return 0
}

//export GetVersion
func GetVersion() *C.char {
	return C.CString("1.0.0")
}

func main() {
	// 必须保留空的 main 函数用于编译 DLL
	fmt.Println("Chimera Remote Port Forward DLL")
	fmt.Println("Use from Flutter/other languages via FFI")
}
