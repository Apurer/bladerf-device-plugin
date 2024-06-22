package main

import (
    "bytes"
    "flag"
    "fmt"
    "io/ioutil"
    "net"
    "os"
    "os/exec"
    "path"
    "strings"
    "sync"
    "time"
    "log"

    "github.com/golang/glog"

    "golang.org/x/net/context"
    "google.golang.org/grpc"

    pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
    NuandVendorID = "2cf0"
    BladeRFProductID = "5246"
    BladeRF2ProductID = "5250" // Assuming same ID, please adjust if necessary
)

const (
    SysfsDevices = "/sys/bus/usb/devices"
    VendorFile   = "idVendor"
    ProductFile  = "idProduct"
)

const (
    socketName   string = "nuandBladeRF"
    resourceName string = "nuand.com/bladerf"
)

type bladeRFDevice struct {
    vid    string
    pid    string
    name   string
    busNum string
    devNum string
    device pluginapi.Device
}

type bladeRFManager struct {
    devices map[string]*bladeRFDevice
}

func NewBladeRFManager() (*bladeRFManager, error) {
    return &bladeRFManager{
        devices: make(map[string]*bladeRFDevice),
    }, nil
}

func GetFileContent(file string) (string, error) {
    if buf, err := ioutil.ReadFile(file); err != nil {
        return "", fmt.Errorf("Can't read file %s", file)
    } else {
        return strings.Trim(string(buf), "\n"), nil
    }
}

func (bladerf *bladeRFManager) discoverBladeRFResources() (bool, error) {
    found := false
    bladerf.devices = make(map[string]*bladeRFDevice)
    glog.Info("Discovering bladeRF Resources")

    usbFiles, err := ioutil.ReadDir(SysfsDevices)
    if err != nil {
        return false, fmt.Errorf("Can't read folder %s", SysfsDevices)
    }

    for _, usbFile := range usbFiles {
        usbID := usbFile.Name()
        if strings.Contains(usbID, ":") {
            continue
        }
        vendorID, err := GetFileContent(path.Join(SysfsDevices, usbID, VendorFile))
        if err != nil {
            return false, err
        }
        productID, err := GetFileContent(path.Join(SysfsDevices, usbID, ProductFile))
        if err != nil {
            return false, err
        }
        productName := "Undefined"
        if strings.EqualFold(vendorID, NuandVendorID) && strings.EqualFold(productID, BladeRF2ProductID) {
            productName = "BladeRF 2.0"
        } else {
            continue
        }
        busnum, err := GetFileContent(path.Join(SysfsDevices, usbID, "busnum"))
        if err != nil {
            return false, err
        }
        devnum, err := GetFileContent(path.Join(SysfsDevices, usbID, "devnum"))
        if err != nil {
            return false, err
        }
        serial, err := GetFileContent(path.Join(SysfsDevices, usbID, "serial"))
        if err != nil {
            return false, err
        }
        healthy := pluginapi.Healthy
        dev := bladeRFDevice{
            vid:     vendorID,
            pid:     productID,
            name:    productName,
            busNum: fmt.Sprintf("%03s", busnum),
            devNum:  fmt.Sprintf("%03s", devnum),
            device: pluginapi.Device{
                ID:     serial,
                Health: healthy},
        }
        bladerf.devices[serial] = &dev
        found = true
    }
    log.Printf("Devices: %v \n", bladerf.devices)
    return found, nil
}

// func (bladerf *bladeRFManager) DownloadBladeRFImages() error {
//     log.Println("Downloading bladeRF images. Be patient")

//     cmd := exec.Command("bladerf-cli", "--flash-firmware", "your_firmware_file_path_here")
//     var out, stderr bytes.Buffer
//     cmd.Stdout = &out
//     cmd.Stderr = &stderr
//     err := cmd.Run()
//     if err != nil {
//         log.Printf("Error: CMD bladerf-cli: %s: %s", err, stderr.String())
//     }
//     return err
// }

// func (bladerf *bladeRFManager) Init() error {
//     glog.Info("Initializing bladeRF Manager")
//     err := bladerf.DownloadBladeRFImages()
//     return err
// }

// Additional methods for Register, ListAndWatch, Allocate, GetPreferredAllocation, PreStartContainer, and GetDevicePluginOptions remain similar, adjusting logging and error messages for bladeRF.
func Register(kubeletEndpoint string, pluginEndpoint, socketName string) error {
    conn, err := grpc.Dial(kubeletEndpoint, grpc.WithInsecure(),
        grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
            return net.DialTimeout("unix", addr, timeout)
        }))
    defer conn.Close()
    if err != nil {
        return fmt.Errorf("bladeRF-device-plugin: cannot connect to kubelet service: %v", err)
    }
    client := pluginapi.NewRegistrationClient(conn)
    reqt := &pluginapi.RegisterRequest{
        Version:      pluginapi.Version,
        Endpoint:     pluginEndpoint,
        ResourceName: resourceName,
    }

    _, err = client.Register(context.Background(), reqt)
    if err != nil {
        return fmt.Errorf("bladeRF-device-plugin: cannot register to kubelet service: %v", err)
    }
    return nil
}

func (bladerf *bladeRFManager) ListAndWatch(emtpy *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
    glog.Info("bladeRF-device-plugin: ListAndWatch started")
    for {
        bladerf.discoverBladeRFResources()
        resp := new(pluginapi.ListAndWatchResponse)
        for _, dev := range bladerf.devices {
            glog.Info("Device listed: ", dev)
            resp.Devices = append(resp.Devices, &dev.device)
        }
        if err := stream.Send(resp); err != nil {
            glog.Errorf("Failed to send response to kubelet: %v\n", err)
        }
        time.Sleep(5 * time.Second)
    }
    return nil
}

func (bladerf *bladeRFManager) Allocate(ctx context.Context, rqt *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
    glog.Info("bladeRF-device-plugin: Allocate started")
    resp := new(pluginapi.AllocateResponse)
    for _, containerRqt := range rqt.ContainerRequests {
        containerResp := new(pluginapi.ContainerAllocateResponse)
        resp.ContainerResponses = append(resp.ContainerResponses, containerResp)
        for _, id := range containerRqt.DevicesIDs {
            if dev, ok := bladerf.devices[id]; ok {
                devPath := path.Join("/dev/bus/usb/", dev.busNum, dev.devNum)
                containerResp.Devices = append(containerResp.Devices, &pluginapi.DeviceSpec{
                    HostPath:      devPath,
                    ContainerPath: devPath,
                    Permissions:   "mrw",
                })
                containerResp.Mounts = append(containerResp.Mounts, &pluginapi.Mount{
                    HostPath:      "/usr/share/nuand/bladerf/",
                    ContainerPath: "/usr/share/nuand/bladerf/",
                    ReadOnly:      true,
                })
            }
        }
        glog.Info("Allocated bladeRF interface ", id)
    }
    return resp, nil
}

func (bladerf *bladeRFManager) GetPreferredAllocation(ctx context.Context, rqt *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
    glog.Info("GetPreferredAllocation: Default behavior")
    return new(pluginapi.PreferredAllocationResponse), nil
}

func (bladerf *bladeRFManager) PreStartContainer(ctx context.Context, rqt *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
    glog.Warning("PreStartContainer() should not be called in bladeRF device plugin")
    return nil, fmt.Errorf("PreStartContainer() is not supported")
}

func (bladerf *bladeRFManager) GetDevicePluginOptions(ctx context.Context, empty *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
    log.Println("GetDevicePluginOptions: Returning empty options for bladeRF")
    return new(pluginapi.DevicePluginOptions), nil
}


func main() {
    flag.Parse()
    log.Println("Starting bladeRF device plugin")

    // Enable logging to stderr (for use with Kubernetes).
    flag.Lookup("logtostderr").Value.Set("true")

    // Create a new bladeRF manager.
    bladerfManager, err := NewBladeRFManager()
    if err != nil {
        glog.Fatalf("Failed to create bladeRF manager: %v", err)
        os.Exit(1)
    }

    // Continuously discover devices until at least one is found.
    found := false
    for !found {
        var err error
        found, err = bladerfManager.discoverBladeRFResources()
        if err != nil {
            glog.Error("Failed to discover bladeRF devices: ", err)
            time.Sleep(5 * time.Second) // Wait before retrying
        }
        if !found {
            glog.Warning("No bladeRF devices detected. Retrying...")
            time.Sleep(5 * time.Second)
        }
    }

    // Initialize the device manager (downloads firmware, sets configurations, etc.).
    // err = bladerfManager.Init()
    // if err != nil {
    //     glog.Errorf("Initialization error: %v", err)
    // }

    // Create the unique endpoint for this plugin.
    pluginEndpoint := fmt.Sprintf("%s-%d.sock", socketName, time.Now().Unix())
    os.Remove(pluginapi.DevicePluginPath + "/" + pluginEndpoint) // Cleanup any existing socket
    lis, err := net.Listen("unix", path.Join(pluginapi.DevicePluginPath, pluginEndpoint))
    if err != nil {
        glog.Fatalf("Failed to listen on the plugin endpoint: %v", err)
        return
    }

    // Start a gRPC server and register the bladeRF manager as a device plugin.
    var opts []grpc.ServerOption
    grpcServer := grpc.NewServer(opts...)
    pluginapi.RegisterDevicePluginServer(grpcServer, bladerfManager)
    go func() {
        if err := grpcServer.Serve(lis); err != nil {
            glog.Fatalf("Failed to serve gRPC server over plugin endpoint: %v", err)
        }
    }()

    // Register the plugin with the kubelet.
    err = Register(pluginapi.KubeletSocket, pluginEndpoint, resourceName)
    if err != nil {
        glog.Fatalf("Failed to register the device plugin: %v", err)
        return
    }
    log.Println("bladeRF device plugin registered successfully with Kubelet")

    // Block on the main thread so that the program doesn't exit
    select {} // Blocks indefinitely
}

