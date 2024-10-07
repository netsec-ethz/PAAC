package paac

import (
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
)

var (
	TestStruct30StringFields []string = []string{"StringField0", "StringField1", "StringField2", "StringField3", "StringField4", "StringField5", "StringField6", "StringField7", "StringField8", "StringField9"}
	TestStruct30IntFields    []string = []string{"IntField0", "IntField1", "IntField2", "IntField3", "IntField4", "IntField5", "IntField6", "IntField7", "IntField8", "IntField9"}
	TestStruct30FloatFields  []string = []string{"FloatField0", "FloatField1", "FloatField2", "FloatField3", "FloatField4", "FloatField5", "FloatField6", "FloatField7", "FloatField8", "FloatField9"}
)

type TestStruct30 struct {
	StringField0 string
	StringField1 string
	StringField2 string
	StringField3 string
	StringField4 string
	StringField5 string
	StringField6 string
	StringField7 string
	StringField8 string
	StringField9 string
	IntField0    int
	IntField1    int
	IntField2    int
	IntField3    int
	IntField4    int
	IntField5    int
	IntField6    int
	IntField7    int
	IntField8    int
	IntField9    int
	FloatField0  float64
	FloatField1  float64
	FloatField2  float64
	FloatField3  float64
	FloatField4  float64
	FloatField5  float64
	FloatField6  float64
	FloatField7  float64
	FloatField8  float64
	FloatField9  float64
}
type TestStruct15 struct {
	StringField0 string
	StringField1 string
	StringField2 string
	StringField3 string
	StringField4 string
	IntField0    int
	IntField1    int
	IntField2    int
	IntField3    int
	IntField4    int
	FloatField0  float64
	FloatField1  float64
	FloatField2  float64
	FloatField3  float64
	FloatField4  float64
}
type TestStruct9 struct {
	StringField0 string
	StringField1 string
	StringField2 string
	IntField0    int
	IntField1    int
	IntField2    int
	FloatField0  float64
	FloatField1  float64
	FloatField2  float64
}

type TestStruct3 struct {
	StringField0 string
	IntField0    int
	FloatField0  float64
}

var (
	Math_ops  []string = []string{"<=", "<", ">", ">=", "=="}
	Logic_ops []string = []string{"&&", "||"}
	Auth_ops  []string = []string{"read", "write"}
	Effects   []string = []string{"deny", "allow"}
)

func NewTestStruct30(j, n int) TestStruct30 {
	var ts TestStruct30
	strs := [10]string{}
	ints := [10]int{}
	floats := [10]float64{}
	for i := 0; i < 10; i++ {
		strs[i] = fmt.Sprintf("Long-Value-For-String-Field-%v-%v", i, j)
		ints[i] = j + i
		floats[i] = float64(n) / float64(j+i+1)
	}
	ts = TestStruct30{
		strs[0], strs[1], strs[2], strs[3], strs[4], strs[5], strs[6], strs[7], strs[8], strs[9],
		ints[0], ints[1], ints[2], ints[3], ints[4], ints[5], ints[6], ints[7], ints[8], ints[9],
		floats[0], floats[1], floats[2], floats[3], floats[4], floats[5], floats[6], floats[7], floats[8], floats[9],
	}
	return ts
}

func NewTestMap(size, j, n int) map[string]any {
	tm := make(map[string]any)

	for i := 0; i < size; i++ {
		tm[TestStruct30StringFields[i]] = fmt.Sprintf("Long-Value-For-String-Field-%v-%v", i, j)
		tm[TestStruct30IntFields[i]] = j + i
		tm[TestStruct30FloatFields[i]] = float64(n) / float64(j+i+1)

	}
	return tm
}

func createDir(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, err
	}
	return os.Create(p)
}

func BuildPolicyCsv(num_rules, eval_size, num_files int, path string) {
	csvFile, err := createDir(path)
	if err != nil {
		log.Fatalf("failed creating file %q: %s", path, err)
	}
	csvwriter := csv.NewWriter(csvFile)

	for i := 0; i < num_rules; i++ {
		sub_s := ""
		for j := 0; j < eval_size; j++ {
			if j != 0 {
				sub_s += " " + RandomChoice(Logic_ops) + " "
			}
			sub_s += fmt.Sprintf("r.sub.%v == 'SomeLongDefaultString'", TestStruct30StringFields[j])
			sub_s += " " + RandomChoice(Logic_ops) + " "
			sub_s += fmt.Sprintf("r.sub.%v %v %v", TestStruct30IntFields[j], RandomChoice(Math_ops), rand.Intn(num_rules))
			sub_s += " " + RandomChoice(Logic_ops) + " "
			sub_s += fmt.Sprintf("r.sub.%v %v %v", TestStruct30FloatFields[j], RandomChoice(Math_ops), 3.1415)

		}
		_ = csvwriter.Write([]string{"p", sub_s, fmt.Sprintf("data%v", i%num_files), RandomChoice(Auth_ops), RandomChoice(Effects)})
	}
	csvwriter.Flush()
	csvFile.Close()
}

// This function takes a slice and returns a random element
func RandomChoice[T any](a []T) T {
	return a[rand.Intn(len(a))]
}
