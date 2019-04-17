package util

import (
	"unsafe"

	lin "github.com/xlab/linmath"
)

type vkTexCubeUniform struct {
	mvp      lin.Mat4x4
	position [12 * 3][4]float32
	attr     [12 * 3][4]float32
}

const vkTexCubeUniformSize = int(unsafe.Sizeof(vkTexCubeUniform{}))

func (u *vkTexCubeUniform) Data() []byte {
	const m = 0x7fffffff
	return (*[m]byte)(unsafe.Pointer(u))[:vkTexCubeUniformSize]
}

var gVertexBufferData = []float32{

	-1.0, 1.0, 1.0, // +Z side
	-1.0, -1.0, 1.0,
	1.0, 1.0, 1.0,
	-1.0, -1.0, 1.0,
	1.0, -1.0, 1.0,
	1.0, 1.0, 1.0,

	-0.5, 1.0, 1.3, // +Z'' side
	-0.5, -1.0, 1.3,
	0.5, 1.0, 1.3,
	-0.5, -1.0, 1.3,
	0.5, -1.0, 1.3,
	0.5, 1.0, 1.3,
}

var gUVBufferData = []float32{
	0.0, 0.0, // +Z side
	0.0, 1.0,
	1.0, 0.0,
	0.0, 1.0,
	1.0, 1.0,
	1.0, 0.0,

	0.0, 0.0, // +Z side
	0.0, 1.0,
	1.0, 0.0,
	0.0, 1.0,
	1.0, 1.0,
	1.0, 0.0,
}