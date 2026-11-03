package constants

import (
	"strconv"
)

type Color uint8

const (
	ColorBlack Color = iota
	ColorRed
	ColorGreen
	ColorYellow
	ColorBlue
	ColorPurple
	ColorCyan
	ColorGray
)

func (c Color) ColorString(s string) string {
	return "\033[3" + strconv.Itoa(int(c)) + "m" + s + "\033[0m"
}

func (c Color) ColorBackground(s string) string {
	return "\033[4" + strconv.Itoa(int(c)) + "m" + s + "\033[0m"
}

const (
	StatusCode200Color = ColorGreen
	StatusCode300Color = ColorGray
	StatusCode400Color = ColorYellow
	StatusCode500Color = ColorRed
)

const (
	MethodGetColor     = ColorBlue
	MethodPostColor    = ColorCyan
	MethodPutColor     = ColorYellow
	MethodPatchColor   = ColorGreen
	MethodDeleteColor  = ColorRed
	MethodHeadColor    = ColorPurple
	MethodOptionsColor = ColorGray
)
