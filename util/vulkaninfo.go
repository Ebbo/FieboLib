package util

import (
	"fmt"
	"github.com/xlab/tablewriter"
	"io/ioutil"
	"log"
	"unsafe"

	vk "github.com/vulkan-go/vulkan"
	"github.com/xlab/linmath"
)

const enableDebug = false

type VulkanDeviceInfo struct {
	gpuDevices []vk.PhysicalDevice

	dbg      vk.DebugReportCallback
	Instance vk.Instance
	Surface  vk.Surface
	Queue    vk.Queue
	Device   vk.Device
}

type VulkanSwapchainInfo struct {
	Device vk.Device

	Swapchains   []vk.Swapchain
	SwapchainLen []uint32

	DisplaySize   vk.Extent2D
	DisplayFormat vk.Format

	Framebuffers []vk.Framebuffer
	DisplayViews []vk.ImageView
}

func (swapchainInfo *VulkanSwapchainInfo) DefaultSwapchain() vk.Swapchain {
	return swapchainInfo.Swapchains[0]
}

func (swapchainInfo *VulkanSwapchainInfo) DefaultSwapchainLen() uint32 {
	return swapchainInfo.SwapchainLen[0]
}

type VulkanBufferInfo struct {
	device        vk.Device
	vertexBuffers []vk.Buffer
}

func (v *VulkanBufferInfo) DefaultVertexBuffer() vk.Buffer {
	return v.vertexBuffers[0]
}

type VulkanGfxPipelineInfo struct {
	device vk.Device

	layout   vk.PipelineLayout
	cache    vk.PipelineCache
	pipeline vk.Pipeline
}

type VulkanRenderInfo struct {
	device vk.Device

	RenderPass vk.RenderPass
	cmdPool    vk.CommandPool
	cmdBuffers []vk.CommandBuffer
	semaphores []vk.Semaphore
	fences     []vk.Fence
}

func (v *VulkanRenderInfo) DefaultFence() vk.Fence {
	return v.fences[0]
}

func (v *VulkanRenderInfo) DefaultSemaphore() vk.Semaphore {
	return v.semaphores[0]
}

func VulkanInit(v *VulkanDeviceInfo, s *VulkanSwapchainInfo,
	r *VulkanRenderInfo, b *VulkanBufferInfo, gfx *VulkanGfxPipelineInfo) {

	clearValues := []vk.ClearValue{
		vk.NewClearValue([]float32{0.098, 0.71, 0.996, 1}),
	}
	for i := range r.cmdBuffers {
		cmdBufferBeginInfo := vk.CommandBufferBeginInfo{
			SType: vk.StructureTypeCommandBufferBeginInfo,
		}
		renderPassBeginInfo := vk.RenderPassBeginInfo{
			SType:       vk.StructureTypeRenderPassBeginInfo,
			RenderPass:  r.RenderPass,
			Framebuffer: s.Framebuffers[i],
			RenderArea: vk.Rect2D{
				Offset: vk.Offset2D{
					X: 0, Y: 0,
				},
				Extent: s.DisplaySize,
			},
			ClearValueCount: 1,
			PClearValues:    clearValues,
		}
		ret := vk.BeginCommandBuffer(r.cmdBuffers[i], &cmdBufferBeginInfo)
		check(ret, "vk.BeginCommandBuffer")

		vk.CmdBeginRenderPass(r.cmdBuffers[i], &renderPassBeginInfo, vk.SubpassContentsInline)
		vk.CmdBindPipeline(r.cmdBuffers[i], vk.PipelineBindPointGraphics, gfx.pipeline)
		offsets := make([]vk.DeviceSize, len(b.vertexBuffers))
		vk.CmdBindVertexBuffers(r.cmdBuffers[i], 0, 1, b.vertexBuffers, offsets)
		vk.CmdDraw(r.cmdBuffers[i], 4, 1, 0, 0)
		vk.CmdEndRenderPass(r.cmdBuffers[i])

		ret = vk.EndCommandBuffer(r.cmdBuffers[i])
		check(ret, "vk.EndCommandBuffer")
	}
	fenceCreateInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
	}
	semaphoreCreateInfo := vk.SemaphoreCreateInfo{
		SType: vk.StructureTypeSemaphoreCreateInfo,
	}
	r.fences = make([]vk.Fence, 1)
	ret := vk.CreateFence(v.Device, &fenceCreateInfo, nil, &r.fences[0])
	check(ret, "vk.CreateFence")
	r.semaphores = make([]vk.Semaphore, 1)
	ret = vk.CreateSemaphore(v.Device, &semaphoreCreateInfo, nil, &r.semaphores[0])
	check(ret, "vk.CreateSemaphore")
}

func VulkanDrawFrame(deviceInfo VulkanDeviceInfo,
	swapchainInfo VulkanSwapchainInfo, renderInfo VulkanRenderInfo) bool {
	var nextIdx uint32

	// Phase 1: vk.AcquireNextImage
	// 			get the framebuffer index we should draw in
	//
	//			N.B. non-infinite timeouts may be not yet implemented
	//			by your Vulkan driver

	err := vk.Error(vk.AcquireNextImage(deviceInfo.Device, swapchainInfo.DefaultSwapchain(),
		vk.MaxUint64, renderInfo.DefaultSemaphore(), vk.NullFence, &nextIdx))
	if err != nil {
		err = fmt.Errorf("vk.AcquireNextImage failed with %s", err)
		log.Println("[WARN]", err)
		return false
	}

	// Phase 2: vk.QueueSubmit
	//			vk.WaitForFences

	vk.ResetFences(deviceInfo.Device, 1, renderInfo.fences)
	submitInfo := []vk.SubmitInfo{{
		SType:              vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount: 1,
		PWaitSemaphores:    renderInfo.semaphores,
		CommandBufferCount: 1,
		PCommandBuffers:    renderInfo.cmdBuffers[nextIdx:],
	}}
	err = vk.Error(vk.QueueSubmit(deviceInfo.Queue, 1, submitInfo, renderInfo.DefaultFence()))
	if err != nil {
		err = fmt.Errorf("vk.QueueSubmit failed with %s", err)
		log.Println("[WARN]", err)
		return false
	}

	const timeoutNano = 10 * 1000 * 1000 * 1000 // 10 sec
	err = vk.Error(vk.WaitForFences(deviceInfo.Device, 1, renderInfo.fences, vk.True, timeoutNano))
	if err != nil {
		err = fmt.Errorf("vk.WaitForFences failed with %s", err)
		log.Println("[WARN]", err)
		return false
	}

	// Phase 3: vk.QueuePresent

	imageIndices := []uint32{nextIdx}
	presentInfo := vk.PresentInfo{
		SType:          vk.StructureTypePresentInfo,
		SwapchainCount: 1,
		PSwapchains:    swapchainInfo.Swapchains,
		PImageIndices:  imageIndices,
	}
	err = vk.Error(vk.QueuePresent(deviceInfo.Queue, &presentInfo))
	if err != nil {
		err = fmt.Errorf("vk.QueuePresent failed with %s", err)
		log.Println("[WARN]", err)
		return false
	}
	return true
}

func (r *VulkanRenderInfo) CreateCommandBuffers(n uint32) error {
	r.cmdBuffers = make([]vk.CommandBuffer, n)
	cmdBufferAllocateInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        r.cmdPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: n,
	}
	err := vk.Error(vk.AllocateCommandBuffers(r.device, &cmdBufferAllocateInfo, r.cmdBuffers))
	if err != nil {
		err = fmt.Errorf("vk.AllocateCommandBuffers failed with %s", err)
		return err
	}
	return nil
}

func CreateRenderer(device vk.Device, displayFormat vk.Format) (VulkanRenderInfo, error) {
	attachmentDescriptions := []vk.AttachmentDescription{{
		Format:         displayFormat,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutColorAttachmentOptimal,
		FinalLayout:    vk.ImageLayoutColorAttachmentOptimal,
	}}
	colorAttachments := []vk.AttachmentReference{{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}}
	subpassDescriptions := []vk.SubpassDescription{{
		PipelineBindPoint:    vk.PipelineBindPointGraphics,
		ColorAttachmentCount: 1,
		PColorAttachments:    colorAttachments,
	}}
	renderPassCreateInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 1,
		PAttachments:    attachmentDescriptions,
		SubpassCount:    1,
		PSubpasses:      subpassDescriptions,
	}
	cmdPoolCreateInfo := vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		Flags:            vk.CommandPoolCreateFlags(vk.CommandPoolCreateResetCommandBufferBit),
		QueueFamilyIndex: 0,
	}
	var r VulkanRenderInfo
	err := vk.Error(vk.CreateRenderPass(device, &renderPassCreateInfo, nil, &r.RenderPass))
	if err != nil {
		err = fmt.Errorf("vk.CreateRenderPass failed with %s", err)
		return r, err
	}
	err = vk.Error(vk.CreateCommandPool(device, &cmdPoolCreateInfo, nil, &r.cmdPool))
	if err != nil {
		err = fmt.Errorf("vk.CreateCommandPool failed with %s", err)
		return r, err
	}
	r.device = device
	return r, nil
}

func NewVulkanDevice(appInfo *vk.ApplicationInfo, window uintptr, instanceExtensions []string, createSurfaceFunc func(interface{}) uintptr) (VulkanDeviceInfo, error) {
	// Phase 1: vk.CreateInstance with vk.InstanceCreateInfo

	existingExtensions := getInstanceExtensions()
	log.Println("[INFO] Instance extensions:", existingExtensions)

	// instanceExtensions := vk.GetRequiredInstanceExtensions()
	if enableDebug {
		instanceExtensions = append(instanceExtensions,
			"VK_EXT_debug_report\x00")
	}

	instanceLayers := []string{}

	instanceCreateInfo := vk.InstanceCreateInfo{
		SType:                   vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo:        appInfo,
		EnabledExtensionCount:   uint32(len(instanceExtensions)),
		PpEnabledExtensionNames: instanceExtensions,
		EnabledLayerCount:       uint32(len(instanceLayers)),
		PpEnabledLayerNames:     instanceLayers,
	}
	var v VulkanDeviceInfo
	err := vk.Error(vk.CreateInstance(&instanceCreateInfo, nil, &v.Instance))
	if err != nil {
		err = fmt.Errorf("vk.CreateInstance failed with %s", err)
		return v, err
	} else {
		vk.InitInstance(v.Instance)
	}

	// Phase 2: vk.CreateAndroidSurface with vk.AndroidSurfaceCreateInfo

	v.Surface = vk.SurfaceFromPointer(createSurfaceFunc(v.Instance))
	// err = vk.Error(vk.CreateWindowSurface(v.Instance, window, nil, &v.Surface))
	if err != nil {
		vk.DestroyInstance(v.Instance, nil)
		err = fmt.Errorf("vkCreateWindowSurface failed with %s", err)
		return v, err
	}
	if v.gpuDevices, err = getPhysicalDevices(v.Instance); err != nil {
		v.gpuDevices = nil
		vk.DestroySurface(v.Instance, v.Surface, nil)
		vk.DestroyInstance(v.Instance, nil)
		return v, err
	}

	existingExtensions = getDeviceExtensions(v.gpuDevices[0])
	log.Println("[INFO] Device extensions:", existingExtensions)

	// Phase 3: vk.CreateDevice with vk.DeviceCreateInfo (a logical device)

	deviceLayers := []string{}

	queueCreateInfos := []vk.DeviceQueueCreateInfo{{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueCount:       1,
		PQueuePriorities: []float32{1.0},
	}}
	deviceExtensions := []string{
		"VK_KHR_swapchain\x00",
	}
	deviceCreateInfo := vk.DeviceCreateInfo{
		SType:                   vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:    uint32(len(queueCreateInfos)),
		PQueueCreateInfos:       queueCreateInfos,
		EnabledExtensionCount:   uint32(len(deviceExtensions)),
		PpEnabledExtensionNames: deviceExtensions,
		EnabledLayerCount:       uint32(len(deviceLayers)),
		PpEnabledLayerNames:     deviceLayers,
	}
	var device vk.Device // we choose the first GPU available for this device
	err = vk.Error(vk.CreateDevice(v.gpuDevices[0], &deviceCreateInfo, nil, &device))
	if err != nil {
		v.gpuDevices = nil
		vk.DestroySurface(v.Instance, v.Surface, nil)
		vk.DestroyInstance(v.Instance, nil)
		err = fmt.Errorf("vk.CreateDevice failed with %s", err)
		return v, err
	} else {
		v.Device = device
		var queue vk.Queue
		vk.GetDeviceQueue(device, 0, 0, &queue)
		v.Queue = queue
	}

	if enableDebug {
		// Phase 4: vk.CreateDebugReportCallback

		dbgCreateInfo := vk.DebugReportCallbackCreateInfo{
			SType:       vk.StructureTypeDebugReportCallbackCreateInfo,
			Flags:       vk.DebugReportFlags(vk.DebugReportErrorBit | vk.DebugReportWarningBit),
			PfnCallback: dbgCallbackFunc,
		}
		var dbg vk.DebugReportCallback
		err = vk.Error(vk.CreateDebugReportCallback(v.Instance, &dbgCreateInfo, nil, &dbg))
		if err != nil {
			err = fmt.Errorf("vk.CreateDebugReportCallback failed with %s", err)
			log.Println("[WARN]", err)
			return v, nil
		}
		v.dbg = dbg
	}
	return v, nil
}

func getInstanceExtensions() (extNames []string) {
	var instanceExtLen uint32
	ret := vk.EnumerateInstanceExtensionProperties("", &instanceExtLen, nil)
	check(ret, "vk.EnumerateInstanceExtensionProperties")
	instanceExt := make([]vk.ExtensionProperties, instanceExtLen)
	ret = vk.EnumerateInstanceExtensionProperties("", &instanceExtLen, instanceExt)
	check(ret, "vk.EnumerateInstanceExtensionProperties")
	for _, ext := range instanceExt {
		ext.Deref()
		extNames = append(extNames,
			vk.ToString(ext.ExtensionName[:]))
	}
	return extNames
}

func getDeviceExtensions(gpu vk.PhysicalDevice) (extNames []string) {
	var deviceExtLen uint32
	ret := vk.EnumerateDeviceExtensionProperties(gpu, "", &deviceExtLen, nil)
	check(ret, "vk.EnumerateDeviceExtensionProperties")
	deviceExt := make([]vk.ExtensionProperties, deviceExtLen)
	ret = vk.EnumerateDeviceExtensionProperties(gpu, "", &deviceExtLen, deviceExt)
	check(ret, "vk.EnumerateDeviceExtensionProperties")
	for _, ext := range deviceExt {
		ext.Deref()
		extNames = append(extNames,
			vk.ToString(ext.ExtensionName[:]))
	}
	return extNames
}

func dbgCallbackFunc(flags vk.DebugReportFlags, objectType vk.DebugReportObjectType,
	object uint64, location uint, messageCode int32, pLayerPrefix string,
	pMessage string, pUserData unsafe.Pointer) vk.Bool32 {

	switch {
	case flags&vk.DebugReportFlags(vk.DebugReportErrorBit) != 0:
		log.Printf("[ERROR %d] %s on layer %s", messageCode, pMessage, pLayerPrefix)
	case flags&vk.DebugReportFlags(vk.DebugReportWarningBit) != 0:
		log.Printf("[WARN %d] %s on layer %s", messageCode, pMessage, pLayerPrefix)
	default:
		log.Printf("[WARN] unknown debug message %d (layer %s)", messageCode, pLayerPrefix)
	}
	return vk.Bool32(vk.False)
}

func getPhysicalDevices(instance vk.Instance) ([]vk.PhysicalDevice, error) {
	var gpuCount uint32
	err := vk.Error(vk.EnumeratePhysicalDevices(instance, &gpuCount, nil))
	if err != nil {
		err = fmt.Errorf("vk.EnumeratePhysicalDevices failed with %s", err)
		return nil, err
	}
	if gpuCount == 0 {
		err = fmt.Errorf("getPhysicalDevice: no GPUs found on the system")
		return nil, err
	}
	gpuList := make([]vk.PhysicalDevice, gpuCount)
	err = vk.Error(vk.EnumeratePhysicalDevices(instance, &gpuCount, gpuList))
	if err != nil {
		err = fmt.Errorf("vk.EnumeratePhysicalDevices failed with %s", err)
		return nil, err
	}
	return gpuList, nil
}

func (v *VulkanDeviceInfo) CreateSwapchain() (VulkanSwapchainInfo, error) {
	gpu := v.gpuDevices[0]

	// Phase 1: vk.GetPhysicalDeviceSurfaceCapabilities
	//			vk.GetPhysicalDeviceSurfaceFormats

	var s VulkanSwapchainInfo
	var surfaceCapabilities vk.SurfaceCapabilities
	err := vk.Error(vk.GetPhysicalDeviceSurfaceCapabilities(gpu, v.Surface, &surfaceCapabilities))
	if err != nil {
		err = fmt.Errorf("vk.GetPhysicalDeviceSurfaceCapabilities failed with %s", err)
		return s, err
	}
	var formatCount uint32
	vk.GetPhysicalDeviceSurfaceFormats(gpu, v.Surface, &formatCount, nil)
	formats := make([]vk.SurfaceFormat, formatCount)
	vk.GetPhysicalDeviceSurfaceFormats(gpu, v.Surface, &formatCount, formats)

	log.Println("[INFO] got", formatCount, "physical device surface formats")

	chosenFormat := -1
	for i := 0; i < int(formatCount); i++ {
		formats[i].Deref()
		if formats[i].Format == vk.FormatB8g8r8a8Unorm ||
			formats[i].Format == vk.FormatR8g8b8a8Unorm {
			chosenFormat = i
			break
		}
	}
	if chosenFormat < 0 {
		err := fmt.Errorf("vk.GetPhysicalDeviceSurfaceFormats not found suitable format")
		return s, err
	}

	// Phase 2: vk.CreateSwapchain
	//			create a swapchain with supported capabilities and format

	surfaceCapabilities.Deref()
	s.DisplaySize = surfaceCapabilities.CurrentExtent
	s.DisplaySize.Deref()
	s.DisplayFormat = formats[chosenFormat].Format
	queueFamily := []uint32{0}
	swapchainCreateInfo := vk.SwapchainCreateInfo{
		SType:           vk.StructureTypeSwapchainCreateInfo,
		Surface:         v.Surface,
		MinImageCount:   surfaceCapabilities.MinImageCount,
		ImageFormat:     formats[chosenFormat].Format,
		ImageColorSpace: formats[chosenFormat].ColorSpace,
		ImageExtent:     surfaceCapabilities.CurrentExtent,
		ImageUsage:      vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit),
		PreTransform:    vk.SurfaceTransformIdentityBit,

		ImageArrayLayers:      1,
		ImageSharingMode:      vk.SharingModeExclusive,
		QueueFamilyIndexCount: 1,
		PQueueFamilyIndices:   queueFamily,
		PresentMode:           vk.PresentModeFifo,
		OldSwapchain:          vk.NullSwapchain,
		Clipped:               vk.False,
	}
	s.Swapchains = make([]vk.Swapchain, 1)
	err = vk.Error(vk.CreateSwapchain(v.Device, &swapchainCreateInfo, nil, &s.Swapchains[0]))
	if err != nil {
		err = fmt.Errorf("vk.CreateSwapchain failed with %s", err)
		return s, err
	}
	s.SwapchainLen = make([]uint32, 1)
	err = vk.Error(vk.GetSwapchainImages(v.Device, s.DefaultSwapchain(), &s.SwapchainLen[0], nil))
	if err != nil {
		err = fmt.Errorf("vk.GetSwapchainImages failed with %s", err)
		return s, err
	}
	for i := range formats {
		formats[i].Free()
	}
	s.Device = v.Device
	return s, nil
}

func (swapchainInfo *VulkanSwapchainInfo) CreateFramebuffers(renderPass vk.RenderPass, depthView vk.ImageView) error {
	// Phase 1: vk.GetSwapchainImages

	var swapchainImagesCount uint32
	err := vk.Error(vk.GetSwapchainImages(swapchainInfo.Device, swapchainInfo.DefaultSwapchain(), &swapchainImagesCount, nil))
	if err != nil {
		err = fmt.Errorf("vk.GetSwapchainImages failed with %s", err)
		return err
	}
	swapchainImages := make([]vk.Image, swapchainImagesCount)
	vk.GetSwapchainImages(swapchainInfo.Device, swapchainInfo.DefaultSwapchain(), &swapchainImagesCount, swapchainImages)

	// Phase 2: vk.CreateImageView
	//			create image view for each swapchain image

	swapchainInfo.DisplayViews = make([]vk.ImageView, len(swapchainImages))
	for i := range swapchainInfo.DisplayViews {
		viewCreateInfo := vk.ImageViewCreateInfo{
			SType:    vk.StructureTypeImageViewCreateInfo,
			Image:    swapchainImages[i],
			ViewType: vk.ImageViewType2d,
			Format:   swapchainInfo.DisplayFormat,
			Components: vk.ComponentMapping{
				R: vk.ComponentSwizzleR,
				G: vk.ComponentSwizzleG,
				B: vk.ComponentSwizzleB,
				A: vk.ComponentSwizzleA,
			},
			SubresourceRange: vk.ImageSubresourceRange{
				AspectMask: vk.ImageAspectFlags(vk.ImageAspectColorBit),
				LevelCount: 1,
				LayerCount: 1,
			},
		}
		err := vk.Error(vk.CreateImageView(swapchainInfo.Device, &viewCreateInfo, nil, &swapchainInfo.DisplayViews[i]))
		if err != nil {
			err = fmt.Errorf("vk.CreateImageView failed with %s", err)
			return err // bail out
		}
	}
	swapchainImages = nil

	// Phase 3: vk.CreateFramebuffer
	//			create a framebuffer from each swapchain image

	swapchainInfo.Framebuffers = make([]vk.Framebuffer, swapchainInfo.DefaultSwapchainLen())
	for i := range swapchainInfo.Framebuffers {
		attachments := []vk.ImageView{
			swapchainInfo.DisplayViews[i], depthView,
		}
		fbCreateInfo := vk.FramebufferCreateInfo{
			SType:           vk.StructureTypeFramebufferCreateInfo,
			RenderPass:      renderPass,
			Layers:          1,
			AttachmentCount: 1, // 2 if has depthView
			PAttachments:    attachments,
			Width:           swapchainInfo.DisplaySize.Width,
			Height:          swapchainInfo.DisplaySize.Height,
		}
		if depthView != vk.NullImageView {
			fbCreateInfo.AttachmentCount = 2
		}
		err := vk.Error(vk.CreateFramebuffer(swapchainInfo.Device, &fbCreateInfo, nil, &swapchainInfo.Framebuffers[i]))
		if err != nil {
			err = fmt.Errorf("vk.CreateFramebuffer failed with %s", err)
			return err // bail out
		}
	}
	return nil
}

func (v VulkanDeviceInfo) CreateBuffers() (VulkanBufferInfo, error) {
	gpu := v.gpuDevices[0]

	// Phase 1: vk.CreateBuffer
	//			create the triangle vertex buffer

	vertexData := linmath.ArrayFloat32([]float32{
		-1, -1, 0,
		1, -1, 0,
		0, 1, 0,
	})
	queueFamilyIdx := []uint32{0}
	bufferCreateInfo := vk.BufferCreateInfo{
		SType:                 vk.StructureTypeBufferCreateInfo,
		Size:                  vk.DeviceSize(vertexData.Sizeof()),
		Usage:                 vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit),
		SharingMode:           vk.SharingModeExclusive,
		QueueFamilyIndexCount: 1,
		PQueueFamilyIndices:   queueFamilyIdx,
	}
	buffer := VulkanBufferInfo{
		vertexBuffers: make([]vk.Buffer, 1),
	}
	err := vk.Error(vk.CreateBuffer(v.Device, &bufferCreateInfo, nil, &buffer.vertexBuffers[0]))
	if err != nil {
		err = fmt.Errorf("vk.CreateBuffer failed with %s", err)
		return buffer, err
	}

	// Phase 2: vk.GetBufferMemoryRequirements
	//			vk.FindMemoryTypeIndex
	// 			assign a proper memory type for that buffer

	var memReq vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(v.Device, buffer.DefaultVertexBuffer(), &memReq)
	memReq.Deref()
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: 0, // see below
	}
	allocInfo.MemoryTypeIndex, _ = vk.FindMemoryTypeIndex(gpu, memReq.MemoryTypeBits,
		vk.MemoryPropertyHostVisibleBit)

	// Phase 3: vk.AllocateMemory
	//			vk.MapMemory
	//			vk.MemCopyFloat32
	//			vk.UnmapMemory
	// 			allocate and map memory for that buffer

	var deviceMemory vk.DeviceMemory
	err = vk.Error(vk.AllocateMemory(v.Device, &allocInfo, nil, &deviceMemory))
	if err != nil {
		err = fmt.Errorf("vk.AllocateMemory failed with %s", err)
		return buffer, err
	}
	var data unsafe.Pointer
	vk.MapMemory(v.Device, deviceMemory, 0, vk.DeviceSize(vertexData.Sizeof()), 0, &data)
	n := vk.Memcopy(data, vertexData.Data())
	if n != vertexData.Sizeof() {
		log.Println("[WARN] failed to copy vertex buffer data")
	}
	vk.UnmapMemory(v.Device, deviceMemory)

	// Phase 4: vk.BindBufferMemory
	//			copy vertex data and bind buffer

	err = vk.Error(vk.BindBufferMemory(v.Device, buffer.DefaultVertexBuffer(), deviceMemory, 0))
	if err != nil {
		err = fmt.Errorf("vk.BindBufferMemory failed with %s", err)
		return buffer, err
	}
	buffer.device = v.Device
	return buffer, err
}

func (buf *VulkanBufferInfo) Destroy() {
	for i := range buf.vertexBuffers {
		vk.DestroyBuffer(buf.device, buf.vertexBuffers[i], nil)
	}
}

func LoadShader(device vk.Device, pathToShader string) (vk.ShaderModule, error) {
	var module vk.ShaderModule
	data, err := ioutil.ReadFile(pathToShader)
	if err != nil {
		err := fmt.Errorf("asset %s not found: %s", pathToShader, err)
		return module, err
	}

	// Phase 1: vk.CreateShaderModule

	shaderModuleCreateInfo := vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uint(len(data)),
		PCode:    repackUint32(data),
	}
	err = vk.Error(vk.CreateShaderModule(device, &shaderModuleCreateInfo, nil, &module))
	if err != nil {
		err = fmt.Errorf("vk.CreateShaderModule failed with %s", err)
		return module, err
	}
	return module, nil
}

func CreateGraphicsPipeline(device vk.Device,
	displaySize vk.Extent2D, renderPass vk.RenderPass) (VulkanGfxPipelineInfo, error) {

	var gfxPipeline VulkanGfxPipelineInfo

	// Phase 1: vk.CreatePipelineLayout
	//			create pipeline layout (empty)

	pipelineLayoutCreateInfo := vk.PipelineLayoutCreateInfo{
		SType: vk.StructureTypePipelineLayoutCreateInfo,
	}
	err := vk.Error(vk.CreatePipelineLayout(device, &pipelineLayoutCreateInfo, nil, &gfxPipeline.layout))
	if err != nil {
		err = fmt.Errorf("vk.CreatePipelineLayout failed with %s", err)
		return gfxPipeline, err
	}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType: vk.StructureTypePipelineDynamicStateCreateInfo,
		// no dynamic state for this demo
	}

	// Phase 2: load shaders and specify shader stages

	vertexShader, err := LoadShader(device, "./util/shader/vert.spv")
	if err != nil { // err has enough info
		return gfxPipeline, err
	}
	defer vk.DestroyShaderModule(device, vertexShader, nil)

	fragmentShader, err := LoadShader(device, "./util/shader/frag.spv")
	if err != nil { // err has enough info
		return gfxPipeline, err
	}
	defer vk.DestroyShaderModule(device, fragmentShader, nil)

	shaderStages := []vk.PipelineShaderStageCreateInfo{
		{
			SType:  vk.StructureTypePipelineShaderStageCreateInfo,
			Stage:  vk.ShaderStageVertexBit,
			Module: vertexShader,
			PName:  "main\x00",
		},
		{
			SType:  vk.StructureTypePipelineShaderStageCreateInfo,
			Stage:  vk.ShaderStageFragmentBit,
			Module: fragmentShader,
			PName:  "main\x00",
		},
	}

	// Phase 3: specify viewport state

	viewports := []vk.Viewport{{
		MinDepth: 0.0,
		MaxDepth: 1.0,
		X:        0,
		Y:        0,
		Width:    float32(displaySize.Width),
		Height:   float32(displaySize.Height),
	}}
	scissors := []vk.Rect2D{{
		Extent: displaySize,
		Offset: vk.Offset2D{
			X: 0, Y: 0,
		},
	}}
	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		PViewports:    viewports,
		ScissorCount:  1,
		PScissors:     scissors,
	}

	// Phase 4: specify multisample state
	//					color blend state
	//					rasterizer state

	sampleMask := []vk.SampleMask{vk.SampleMask(vk.MaxUint32)}
	multisampleState := vk.PipelineMultisampleStateCreateInfo{
		SType:                vk.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples: vk.SampleCount1Bit,
		SampleShadingEnable:  vk.False,
		PSampleMask:          sampleMask,
	}
	attachmentStates := []vk.PipelineColorBlendAttachmentState{{
		ColorWriteMask: vk.ColorComponentFlags(
			vk.ColorComponentRBit | vk.ColorComponentGBit |
				vk.ColorComponentBBit | vk.ColorComponentABit,
		),
		BlendEnable: vk.False,
	}}
	colorBlendState := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		LogicOpEnable:   vk.False,
		LogicOp:         vk.LogicOpCopy,
		AttachmentCount: 1,
		PAttachments:    attachmentStates,
	}
	rasterState := vk.PipelineRasterizationStateCreateInfo{
		SType:                   vk.StructureTypePipelineRasterizationStateCreateInfo,
		DepthClampEnable:        vk.False,
		RasterizerDiscardEnable: vk.False,
		PolygonMode:             vk.PolygonModeFill,
		CullMode:                vk.CullModeFlags(vk.CullModeNone),
		FrontFace:               vk.FrontFaceClockwise,
		DepthBiasEnable:         vk.False,
		LineWidth:               1,
	}

	// Phase 5: specify input assembly state
	//					vertex input state and attributes

	inputAssemblyState := vk.PipelineInputAssemblyStateCreateInfo{
		SType:                  vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               vk.PrimitiveTopologyTriangleStrip,
		PrimitiveRestartEnable: vk.True,
	}
	vertexInputBindings := []vk.VertexInputBindingDescription{{
		Binding:   0,
		Stride:    3 * 4, // 4 = sizeof(float32)
		InputRate: vk.VertexInputRateVertex,
	}}
	vertexInputAttributes := []vk.VertexInputAttributeDescription{{
		Binding:  0,
		Location: 0,
		Format:   vk.FormatR32g32b32Sfloat,
		Offset:   0,
	}}
	vertexInputState := vk.PipelineVertexInputStateCreateInfo{
		SType:                           vk.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		PVertexBindingDescriptions:      vertexInputBindings,
		VertexAttributeDescriptionCount: 1,
		PVertexAttributeDescriptions:    vertexInputAttributes,
	}

	// Phase 5: vk.CreatePipelineCache
	//			vk.CreateGraphicsPipelines

	pipelineCacheInfo := vk.PipelineCacheCreateInfo{
		SType: vk.StructureTypePipelineCacheCreateInfo,
	}
	err = vk.Error(vk.CreatePipelineCache(device, &pipelineCacheInfo, nil, &gfxPipeline.cache))
	if err != nil {
		err = fmt.Errorf("vk.CreatePipelineCache failed with %s", err)
		return gfxPipeline, err
	}
	pipelineCreateInfos := []vk.GraphicsPipelineCreateInfo{{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          2, // vert + frag
		PStages:             shaderStages,
		PVertexInputState:   &vertexInputState,
		PInputAssemblyState: &inputAssemblyState,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterState,
		PMultisampleState:   &multisampleState,
		PColorBlendState:    &colorBlendState,
		PDynamicState:       &dynamicState,
		Layout:              gfxPipeline.layout,
		RenderPass:          renderPass,
	}}
	pipelines := make([]vk.Pipeline, 1)
	err = vk.Error(vk.CreateGraphicsPipelines(device,
		gfxPipeline.cache, 1, pipelineCreateInfos, nil, pipelines))
	if err != nil {
		err = fmt.Errorf("vk.CreateGraphicsPipelines failed with %s", err)
		return gfxPipeline, err
	}
	gfxPipeline.pipeline = pipelines[0]
	gfxPipeline.device = device
	return gfxPipeline, nil
}

func getInstanceLayers() (layerNames []string) {
	var instanceLayerLen uint32
	err := vk.EnumerateInstanceLayerProperties(&instanceLayerLen, nil)
	orPanic(err)
	instanceLayers := make([]vk.LayerProperties, instanceLayerLen)
	err = vk.EnumerateInstanceLayerProperties(&instanceLayerLen, instanceLayers)
	orPanic(err)
	for _, layer := range instanceLayers {
		layer.Deref()
		layerNames = append(layerNames,
			vk.ToString(layer.LayerName[:]))
	}
	return layerNames
}

func getDeviceLayers(gpu vk.PhysicalDevice) (layerNames []string) {
	var deviceLayerLen uint32
	err := vk.EnumerateDeviceLayerProperties(gpu, &deviceLayerLen, nil)
	orPanic(err)
	deviceLayers := make([]vk.LayerProperties, deviceLayerLen)
	err = vk.EnumerateDeviceLayerProperties(gpu, &deviceLayerLen, deviceLayers)
	orPanic(err)
	for _, layer := range deviceLayers {
		layer.Deref()
		layerNames = append(layerNames,
			vk.ToString(layer.LayerName[:]))
	}
	return layerNames
}

func PrintInfo(deviceInfo *VulkanDeviceInfo) {
	var gpuProperties vk.PhysicalDeviceProperties
	vk.GetPhysicalDeviceProperties(deviceInfo.gpuDevices[0], &gpuProperties)
	gpuProperties.Deref()

	table := tablewriter.CreateTable()
	table.UTF8Box()
	table.AddTitle("VULKAN PROPERTIES AND SURFACE CAPABILITES")
	table.AddRow("Physical Device Name", vk.ToString(gpuProperties.DeviceName[:]))
	table.AddRow("Physical Device Vendor", fmt.Sprintf("%x", gpuProperties.VendorID))
	if gpuProperties.DeviceType != vk.PhysicalDeviceTypeOther {
		table.AddRow("Physical Device Type", physicalDeviceType(gpuProperties.DeviceType))
	}
	table.AddRow("Physical GPUs", len(deviceInfo.gpuDevices))
	table.AddRow("API Version", vk.Version(gpuProperties.ApiVersion))
	table.AddRow("API Version Supported", vk.Version(gpuProperties.ApiVersion))
	table.AddRow("Driver Version", vk.Version(gpuProperties.DriverVersion))

	if deviceInfo.Surface != vk.NullSurface {
		var surfaceCapabilities vk.SurfaceCapabilities
		vk.GetPhysicalDeviceSurfaceCapabilities(deviceInfo.gpuDevices[0], deviceInfo.Surface, &surfaceCapabilities)
		surfaceCapabilities.Deref()
		surfaceCapabilities.CurrentExtent.Deref()
		surfaceCapabilities.MinImageExtent.Deref()
		surfaceCapabilities.MaxImageExtent.Deref()

		table.AddSeparator()
		table.AddRow("Image count", fmt.Sprintf("%d - %d",
			surfaceCapabilities.MinImageCount, surfaceCapabilities.MaxImageCount))
		table.AddRow("Array layers", fmt.Sprintf("%d",
			surfaceCapabilities.MaxImageArrayLayers))
		table.AddRow("Image size (current)", fmt.Sprintf("%dx%d",
			surfaceCapabilities.CurrentExtent.Width, surfaceCapabilities.CurrentExtent.Height))
		table.AddRow("Image size (extent)", fmt.Sprintf("%dx%d - %dx%d",
			surfaceCapabilities.MinImageExtent.Width, surfaceCapabilities.MinImageExtent.Height,
			surfaceCapabilities.MaxImageExtent.Width, surfaceCapabilities.MaxImageExtent.Height))
		table.AddRow("Usage flags", fmt.Sprintf("%02x",
			surfaceCapabilities.SupportedUsageFlags))
		table.AddRow("Current transform", fmt.Sprintf("%02x",
			surfaceCapabilities.CurrentTransform))
		table.AddRow("Allowed transforms", fmt.Sprintf("%02x",
			surfaceCapabilities.SupportedTransforms))
		var formatCount uint32
		vk.GetPhysicalDeviceSurfaceFormats(deviceInfo.gpuDevices[0], deviceInfo.Surface, &formatCount, nil)
		table.AddRow("Surface formats", fmt.Sprintf("%d of %d", formatCount, vk.FormatRangeSize))
		table.AddSeparator()
	}

	table.AddRow("INSTANCE EXTENSIONS", "")
	instanceExt := getInstanceExtensions()
	for i, extName := range instanceExt {
		table.AddRow(i+1, extName)
	}

	table.AddSeparator()
	table.AddRow("DEVICE EXTENSIONS", "")
	deviceExt := getDeviceExtensions(deviceInfo.gpuDevices[0])
	for i, extName := range deviceExt {
		table.AddRow(i+1, extName)
	}

	instanceLayers := getInstanceLayers()
	if len(instanceLayers) > 0 {
		table.AddSeparator()
		table.AddRow("INSTANCE LAYERS")
		for i, layerName := range instanceLayers {
			table.AddRow(i+1, layerName)
		}
	}

	deviceLayers := getDeviceLayers(deviceInfo.gpuDevices[0])
	if len(deviceLayers) > 0 {
		table.AddSeparator()
		table.AddRow("DEVICE LAYERS")
		for i, layerName := range deviceLayers {
			table.AddRow(i+1, layerName)
		}
	}

	fmt.Println("\n\n" + table.Render())
}


func physicalDeviceType(gpuType vk.PhysicalDeviceType) string {
	switch gpuType {
	case vk.PhysicalDeviceTypeIntegratedGpu:
		return "Integrated GPU"
	case vk.PhysicalDeviceTypeDiscreteGpu:
		return "Discrete GPU"
	case vk.PhysicalDeviceTypeVirtualGpu:
		return "Virtual GPU"
	case vk.PhysicalDeviceTypeCpu:
		return "CPU"
	case vk.PhysicalDeviceTypeOther:
		return "Other"
	default:
		return "Unknown"
	}
}

func (pipelineInfo *VulkanGfxPipelineInfo) Destroy() {
	if pipelineInfo == nil {
		return
	}
	vk.DestroyPipeline(pipelineInfo.device, pipelineInfo.pipeline, nil)
	vk.DestroyPipelineCache(pipelineInfo.device, pipelineInfo.cache, nil)
	vk.DestroyPipelineLayout(pipelineInfo.device, pipelineInfo.layout, nil)
}

func (swapchainInfo *VulkanSwapchainInfo) Destroy() {
	for i := uint32(0); i < swapchainInfo.DefaultSwapchainLen(); i++ {
		vk.DestroyFramebuffer(swapchainInfo.Device, swapchainInfo.Framebuffers[i], nil)
		vk.DestroyImageView(swapchainInfo.Device, swapchainInfo.DisplayViews[i], nil)
	}
	swapchainInfo.Framebuffers = nil
	swapchainInfo.DisplayViews = nil
	for i := range swapchainInfo.Swapchains {
		vk.DestroySwapchain(swapchainInfo.Device, swapchainInfo.Swapchains[i], nil)
	}
}

func DestroyInOrder(deviceInfo *VulkanDeviceInfo, swapchainInfo *VulkanSwapchainInfo,
	renderInfo *VulkanRenderInfo, bufferInfo *VulkanBufferInfo, pipelineInfo *VulkanGfxPipelineInfo) {

	vk.FreeCommandBuffers(deviceInfo.Device, renderInfo.cmdPool, uint32(len(renderInfo.cmdBuffers)), renderInfo.cmdBuffers)
	renderInfo.cmdBuffers = nil

	vk.DestroyCommandPool(deviceInfo.Device, renderInfo.cmdPool, nil)
	vk.DestroyRenderPass(deviceInfo.Device, renderInfo.RenderPass, nil)

	swapchainInfo.Destroy()
	pipelineInfo.Destroy()
	bufferInfo.Destroy()
	vk.DestroyDevice(deviceInfo.Device, nil)
	if deviceInfo.dbg != vk.NullDebugReportCallback {
		vk.DestroyDebugReportCallback(deviceInfo.Instance, deviceInfo.dbg, nil)
	}
	vk.DestroyInstance(deviceInfo.Instance, nil)
}

func (deviceInfo *VulkanDeviceInfo) Destroy() {
	if deviceInfo == nil {
		return
	}
	deviceInfo.gpuDevices = nil
	vk.DestroyDevice(deviceInfo.Device, nil)
	vk.DestroyInstance(deviceInfo.Instance, nil)
}