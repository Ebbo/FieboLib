package main

import (
	"github.com/ebbo/vulkTest/util"
	"github.com/vulkan-go/glfw/v3.3/glfw"
	vk "github.com/vulkan-go/vulkan"
	"github.com/xlab/closer"
	"log"
	"time"
)

var (
	deviceInfo   util.VulkanDeviceInfo
	swapChainInfo   util.VulkanSwapchainInfo
	renderInfo   util.VulkanRenderInfo
	bufferInfo   util.VulkanBufferInfo
	pipelineInfo util.VulkanGfxPipelineInfo
)

 var appInfo = &vk.ApplicationInfo{
	SType:              vk.StructureTypeApplicationInfo,
	ApiVersion:         vk.MakeVersion(1, 0, 0),
	ApplicationVersion: vk.MakeVersion(1, 0, 0),
	PApplicationName:   "VulkanInfo\x00",
	PEngineName:        "vulkango.com\x00",
}


 func recreateSwapChain() {
 	vk.DeviceWaitIdle(deviceInfo.Device)
 	swapChainInfo.Destroy()
	deviceInfo.CreateSwapchain()
	util.CreateRenderer(deviceInfo.Device, swapChainInfo.DisplayFormat)
	swapChainInfo.CreateFramebuffers(renderInfo.RenderPass, nil)
	deviceInfo.CreateBuffers()
	util.CreateGraphicsPipeline(deviceInfo.Device, swapChainInfo.DisplaySize, renderInfo.RenderPass)
	renderInfo.CreateCommandBuffers(swapChainInfo.DefaultSwapchainLen())
 }
func main() {
	procAddr := glfw.GetVulkanGetInstanceProcAddress()
	if procAddr == nil {
		panic("GetInstanceProcAddress is nil")
	}
	vk.SetGetInstanceProcAddr(procAddr)

	orPanic(glfw.Init())
	orPanic(vk.Init())
	defer closer.Close()

	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI)
	glfw.WindowHint(glfw.Resizable,glfw.True)

	window, err := glfw.CreateWindow(500, 500, "Ebbo's Vulkan Test :D", nil, nil)
	orPanic(err)

	createSurface := func(instance interface{}) uintptr {
		surface, err := window.CreateWindowSurface(instance, nil)
		orPanic(err)
		return surface
	}

	deviceInfo, err = util.NewVulkanDevice(appInfo,
		window.GLFWWindow(),
		window.GetRequiredInstanceExtensions(),
		createSurface)

	orPanic(err)
	swapChainInfo, err = deviceInfo.CreateSwapchain()
	orPanic(err)
	renderInfo, err = util.CreateRenderer(deviceInfo.Device, swapChainInfo.DisplayFormat)
	orPanic(err)
	err = swapChainInfo.CreateFramebuffers(renderInfo.RenderPass, nil)
	orPanic(err)
	bufferInfo, err = deviceInfo.CreateBuffers()
	orPanic(err)
	pipelineInfo, err = util.CreateGraphicsPipeline(deviceInfo.Device, swapChainInfo.DisplaySize, renderInfo.RenderPass)
	orPanic(err)
	log.Println("[INFO] swapchain lengths:", swapChainInfo.SwapchainLen)
	err = renderInfo.CreateCommandBuffers(swapChainInfo.DefaultSwapchainLen())
	orPanic(err)

	doneC := make(chan struct{}, 2)
	exitC := make(chan struct{}, 2)
	defer closer.Bind(func() {
		exitC <- struct{}{}
		<-doneC
		log.Println("Bye!")
	})


	util.PrintInfo(&deviceInfo)
	util.VulkanInit(&deviceInfo, &swapChainInfo, &renderInfo, &bufferInfo, &pipelineInfo)

	fpsTicker := time.NewTicker(time.Second / 30)
	for {
		select {
		case <-exitC:
			util.DestroyInOrder(&deviceInfo, &swapChainInfo, &renderInfo, &bufferInfo, &pipelineInfo)
			window.Destroy()
			glfw.Terminate()
			fpsTicker.Stop()
			doneC <- struct{}{}
			return
		case <-fpsTicker.C:
			if window.ShouldClose() {
				exitC <- struct{}{}
				continue
			}
			glfw.PollEvents()
			util.VulkanDrawFrame(deviceInfo,swapChainInfo,renderInfo)
		}
	}
}

func orPanic(err interface{}) {
	switch v := err.(type) {
	case error:
		if v != nil {
			panic(err)
		}
	case vk.Result:
		if err := vk.Error(v); err != nil {
			panic(err)
		}
	case bool:
		if !v {
			panic("condition failed: != true")
		}
	}
}