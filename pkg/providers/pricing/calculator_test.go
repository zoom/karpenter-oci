/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pricing

import (
	"context"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/assert"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"testing"
	"time"
)

func TestPrice(t *testing.T) {

	gpuA10 := "BM.GPU.A10.4"
	gpuNum := 4

	b1 := "VM.Standard.B1.16"
	var b1CpuNum float32 = 16.0

	b1_1 := "VM.Standard.B1.1"
	var b1_1CpuNum float32 = 1

	denseIO_E5 := "BM.DenseIO.E5.128"
	var denseIO_E5_CpuNum float32 = 128.0
	var denseIO_E5_Mem float32 = 1536
	var denseIO_E5_Disk float32 = 83558.4

	standardE2 := "VM.Standard.E2.1"
	var standardE2_CpuNum float32 = 1
	var standardE2_Mem float32 = 1

	standardE4 := "VM.Standard.E4.Flex"
	var standardE4_CpuNum float32 = 1
	var standardE4_Mem float32 = 8

	standardE6 := "VM.Standard.E6.Flex"
	var standardE6_CpuNum float32 = 1
	var standardE6_Mem float32 = 12

	a1 := "VM.Standard.A1.Flex"
	var a1_CpuNum float32 = 1
	var a1_Mem float32 = 6

	a2 := "VM.Standard.A2.Flex"
	var a2_CpuNum float32 = 1
	var a2_Mem float32 = 6

	standard3 := "VM.Standard3.Flex"
	var standard3_CpuNum float32 = 1
	var standard3_Mem float32 = 8

	denseIO2 := "VM.DenseIO2.16"
	var denseIO2_CpuNum float32 = 16
	var denseIO2_Mem float32 = 240

	testCases := []struct {
		Shape *core.Shape
		Price float32
	}{
		{
			Shape: &core.Shape{
				Shape: &gpuA10,
				Gpus:  &gpuNum,
			},
			Price: 8.0,
		},
		{
			Shape: &core.Shape{
				Shape: &b1,
				Ocpus: &b1CpuNum,
			},
			Price: 1.0208,
		},
		{
			Shape: &core.Shape{
				Shape: &b1_1,
				Ocpus: &b1_1CpuNum,
			},
			Price: 0.0638,
		},
		{
			Shape: &core.Shape{
				Shape:                    &denseIO_E5,
				Ocpus:                    &denseIO_E5_CpuNum,
				MemoryInGBs:              &denseIO_E5_Mem,
				LocalDisksTotalSizeInGBs: &denseIO_E5_Disk,
			},
			Price: 11.90592,
		},
		{
			Shape: &core.Shape{
				Shape:       &standardE2,
				Ocpus:       &standardE2_CpuNum,
				MemoryInGBs: &standardE2_Mem,
			},
			Price: 0.03,
		},
		{
			Shape: &core.Shape{
				Shape:       &standardE4,
				Ocpus:       &standardE4_CpuNum,
				MemoryInGBs: &standardE4_Mem,
			},
			Price: 0.037,
		},
		{
			Shape: &core.Shape{
				Shape:       &standardE6,
				Ocpus:       &standardE6_CpuNum,
				MemoryInGBs: &standardE6_Mem,
			},
			Price: 0.054,
		},

		{
			Shape: &core.Shape{
				Shape:       &a1,
				Ocpus:       &a1_CpuNum,
				MemoryInGBs: &a1_Mem,
			},
			Price: 0,
		},
		{
			Shape: &core.Shape{
				Shape:       &a2,
				Ocpus:       &a2_CpuNum,
				MemoryInGBs: &a2_Mem,
			},
			Price: 0.026,
		},
		{
			Shape: &core.Shape{
				Shape:       &standard3,
				Ocpus:       &standard3_CpuNum,
				MemoryInGBs: &standard3_Mem,
			},
			Price: 0.347125,
		},

		{
			Shape: &core.Shape{
				Shape:       &denseIO2,
				Ocpus:       &denseIO2_CpuNum,
				MemoryInGBs: &denseIO2_Mem,
			},
			Price: 3.8992,
		},
	}

	endpint := "https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/"
	//endpint := "http://localhost:8888/price.json"

	syncer := NewDefaultProvider(context.Background(), endpint)

	time.Sleep(18 * time.Second)

	for _, tc := range testCases {

		wrapShape := &internalmodel.WrapShape{
			Shape: *tc.Shape,
		}

		if tc.Shape.Ocpus != nil {
			wrapShape.CalcCpu = int64(*tc.Shape.Ocpus * 2)
		}
		if tc.Shape.MemoryInGBs != nil {
			wrapShape.CalMemInGBs = int64(*tc.Shape.MemoryInGBs)

		}
		price := syncer.Price(wrapShape)
		if !assert.InDelta(t, price, tc.Price, 1e-6, "floats should be close") {
			t.Errorf("%v,expected: %+v, actual: %+v", *tc.Shape.Shape, tc.Price, price)
		}
	}
}
