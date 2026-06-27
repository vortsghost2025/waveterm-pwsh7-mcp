package main

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/wavetermdev/waveterm/tsunami/app"
	"github.com/wavetermdev/waveterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{
	Title:     "Task Runner",
	ShortDesc: "Run Taskfile tasks and npm commands with a friendly UI",
}

func discoverTasks() []string {
	tasks := []string{"build", "dev", "generate", "test", "lint", "preview"}
	return tasks
}

func discoverNpmScripts() []string {
	npmScripts := []string{"dev", "build", "test", "lint", "preview"}
	return npmScripts
}

func runTask(task string) string {
	cmd := exec.Command("task", task)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Run()
	return out.String()
}

func runNpmScript(script string) string {
	cmd := exec.Command("npm", "run", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Run()
	return out.String()
}

var App = app.DefineComponent("App", func(_ any) any {
	outputAtom := app.UseLocal("")
	isRunningAtom := app.UseLocal(false)
	currentTaskAtom := app.UseLocal("")

	tasks := discoverTasks()
	npmScripts := discoverNpmScripts()

	runHandler := func(task string) {
		isRunningAtom.Set(true)
		currentTaskAtom.Set(task)
		outputAtom.Set("Running " + task + "...\n")

		go func() {
			var result string
			if strings.HasPrefix(task, "npm:") {
				result = runNpmScript(strings.TrimPrefix(task, "npm:"))
			} else {
				result = runTask(task)
			}
			outputAtom.Set(result)
			isRunningAtom.Set(false)
			currentTaskAtom.Set("")
		}()
	}

	return vdom.H("div", map[string]any{
		"className": "h-screen flex flex-col font-sans bg-primary text-primary",
	},
		vdom.H("div", map[string]any{
			"className": "p-4 border-b border-border",
		},
			vdom.H("h1", map[string]any{
				"className": "text-xl font-bold",
			}, "Task Runner"),
			vdom.H("p", map[string]any{
				"className": "text-sm text-secondary",
			}, "Run Taskfile tasks and npm scripts"),
		),

		vdom.H("div", map[string]any{
			"className": "flex-1 flex overflow-hidden",
		},
			vdom.H("div", map[string]any{
				"className": "w-64 border-r border-border p-4 overflow-y-auto",
			},
				vdom.H("div", map[string]any{
					"className": "mb-4",
				},
					vdom.H("h2", map[string]any{
						"className": "text-sm font-semibold mb-2 text-secondary",
					}, "Taskfile Tasks"),
					vdom.ForEach(tasks, func(task string, _ int) any {
						return vdom.H("button", map[string]any{
							"className": "w-full p-2 mb-1 text-left text-sm rounded border border-border hover:bg-accent transition-colors cursor-pointer",
							"onClick": func() { runHandler(task) },
							"disabled": isRunningAtom.Get(),
						}, task)
					}),
				),
				vdom.H("div", map[string]any{
					"className": "mb-4",
				},
					vdom.H("h2", map[string]any{
						"className": "text-sm font-semibold mb-2 text-secondary",
					}, "npm Scripts"),
					vdom.ForEach(npmScripts, func(script string, _ int) any {
						return vdom.H("button", map[string]any{
							"className": "w-full p-2 mb-1 text-left text-sm rounded border border-border hover:bg-accent transition-colors cursor-pointer",
							"onClick": func() { runHandler("npm:" + script) },
							"disabled": isRunningAtom.Get(),
						}, "npm "+script)
					}),
				),
			),
			vdom.H("div", map[string]any{
				"className": "flex-1 flex flex-col p-4 overflow-hidden",
			},
				vdom.H("div", map[string]any{
					"className": "flex-1 bg-tertiary rounded font-mono text-sm overflow-y-auto",
				},
					vdom.H("pre", map[string]any{
						"className": "p-4 h-full overflow-y-auto",
						"style":    "white-space: pre-wrap; word-wrap: break-word;",
					}, outputAtom.Get()),
				),
				vdom.If(isRunningAtom.Get(),
					vdom.H("div", map[string]any{
						"className": "mt-2 text-yellow-500 text-sm",
					}, "Running "+currentTaskAtom.Get()+"..."),
				),
			),
		),
	)
},
)

func main() {
	app.Run(&AppMeta, &App)
}