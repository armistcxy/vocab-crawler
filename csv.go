package main

import (
	"fmt"
	"log"

	"github.com/xuri/excelize/v2"
)

func CreateSheet(name string, fields ...string) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	setField(f, fields)

	if err := f.SaveAs(name + ".xlsx"); err != nil {
		log.Println(err)
	}
}

func setField(f *excelize.File, fields []string) {
	for i, field := range fields {
		pos := convertIndexToFieldPostion(i)
		if err := f.SetCellValue("Sheet1", pos, field); err != nil {
			log.Println(err)
		}
	}
}

func convertIndexToFieldPostion(index int) string {
	col := string(byte('A' + index))
	return fmt.Sprintf("%s%d", col, 1)
}
