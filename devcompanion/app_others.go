//go:build !darwin
package main

func (a *App) setInteractiveShapeNative(rects []float64, fullWindow bool) {}
func (a *App) setupNativeTray()                                            {}
func (a *App) showTalkButton()                                             {}
func (a *App) hideTalkButton()                                             {}
func (a *App) repositionTalkButton()                                       {}
