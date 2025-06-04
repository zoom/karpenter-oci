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
	"testing"
)

func TestPriceListSyncer_Start(t *testing.T) {

	endpint := "https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/"

	var period int64 = 60 * 2

	syncer := NewPriceListSyncer(endpint, period, true)
	_ = syncer.Start()

	//time.Sleep(20 * time.Second)

	/*
	   "VM.Standard2.24",
	   "VM.Standard.E2.8",
	   "VM.Standard.E2.1.Micro",
	   "BM.DenseIO.E5.128",
	   "BM.Standard3.64",
	   "BM.HPC2.36",
	   "BM.Standard.B1.44",
	   "VM.Standard2.8",
	   "BM.Standard.E5.192",
	   "VM.Standard2.4",
	   "VM.Standard.B1.1",
	   "VM.GPU3.4",
	   "VM.Standard1.4",
	   "VM.Standard1.16",
	   "VM.GPU.A10.1",
	   "BM.Standard2.52",
	   "BM.DenseIO2.52",
	   "VM.Standard.E2.2",
	   "VM.DenseIO2.24",
	   "VM.Standard1.2",
	   "BM.GPU.A10.4",
	   "VM.GPU3.2",
	   "BM.Standard.A1.160",
	   "VM.Standard.B1.4",
	   "VM.Standard.B1.8",
	   "VM.GPU.A10.2",
	   "VM.GPU3.1",
	   "VM.DenseIO2.16",
	   "BM.Standard.E6.256",
	   "BM.Optimized3.36",
	   "BM.DenseIO.E4.128",
	   "BM.Standard.E3.128",
	   "VM.Standard2.16",
	   "BM.Standard.E2.64",
	   "BM.GPU.B4.8",
	   "BM.Standard1.36",
	   "VM.Standard2.1",
	   "VM.Standard2.2",
	   "VM.Standard.E2.1",
	   "VM.Standard1.8",
	   "VM.GPU2.1",
	   "VM.Standard.B1.16",
	   "BM.GPU3.8",
	   "BM.GPU2.2",
	   "BM.Standard.E4.128",
	   "VM.Standard.E2.4",
	   "VM.DenseIO2.8",
	   "VM.Standard1.1",
	   "VM.Standard.B1.2",
	*/

	testcases := []struct {
		Shape string
		Found bool
	}{
		{"VM.Standard2.24", true},
		{"VM.Standard.E2.8", true},
		{"VM.Standard.E2.1.Micro", true},
		{"BM.DenseIO.E5.128", true},
		{"BM.Standard3.64", true},
		{"BM.HPC2.36", true},
		{"BM.Standard.B1.44", true},
		{"VM.Standard2.8", true},
		{"BM.Standard.E5.192", true},
		{"VM.Standard2.4", true},
		{"VM.Standard.B1.1", true},
		{"VM.GPU3.4", true},
		{"VM.Standard1.4", true},
		{"VM.Standard1.16", true},
		{"VM.GPU.A10.1", true},
		{"BM.Standard2.52", true},
		{"BM.DenseIO2.52", true},
		{"VM.Standard.E2.2", true},
		{"VM.DenseIO2.24", true},
		{"VM.Standard1.2", true},
		{"BM.GPU.A10.4", true},
		{"VM.GPU3.2", true},
		{"BM.Standard.A1.,160", true},
		{"VM.Standard.B1.4", true},
		{"VM.Standard.B1.8", true},
		{"VM.GPU.A10.2", true},
		{"VM.GPU3.1", true},
		{"BM.Standard.E6.256", true},
		{"BM.Optimized3.36", true},
		{"BM.DenseIO.E4.128", true},
		{"BM.Standard.E3.128", true},
		{"VM.Standard2.16", true},
		{"BM.Standard.E2.64", true},
		{"BM.GPU.B4.8", false},
		{"BM.Standard1.36", true},
		{"VM.Standard2.1", true},
		{"VM.Standard2.2", true},
		{"VM.Standard.E2.1", true},
		{"VM.Standard1.8", true},
		{"VM.GPU2.1", true},
		{"VM.Standard.B1.16", true},
		{"BM.GPU3.8", true},
		{"BM.GPU2.2", true},
		{"BM.Standard.E4.128", true},
		{"VM.Standard.E2.4", true},
		{"VM.DenseIO2.8", true},
		{"VM.Standard1.1", true},
		{"VM.Standard.B1.2", true},
	}

	for _, testcase := range testcases {
		item := syncer.PriceCatalog.FindPriceItems(testcase.Shape)
		find := len(item) != 0
		if testcase.Found != find {
			t.Errorf("it is %v that %s can be found, but actual %v\n", testcase.Found, testcase.Shape, find)
		}
	}

}
