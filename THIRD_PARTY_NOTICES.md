# Third-party notices

Batch Rename is Copyright (c) 2026 Lu Yifei and is distributed under the MIT License. See `LICENSE` for the complete terms. Third-party components retain their own copyrights and licenses.

Runtime components included in the application:

| Component | License | Source |
|---|---|---|
| Vue 3, @vue/shared, @vue/reactivity, @vue/runtime-core, @vue/runtime-dom | MIT | https://github.com/vuejs/core |
| TOS App Kit: tos-app-core, tos-app-ui-kit, Go module | MIT | https://github.com/LuYifei2011/tos-app-kit |
| Lucide Vue (@lucide/vue) | ISC | https://github.com/lucide-icons/lucide |
| Go runtime and standard library | BSD-3-Clause | https://go.dev/LICENSE |
| golang.org/x/sys | BSD-3-Clause | https://go.googlesource.com/sys |

The package build copies the corresponding license files into `/usr/local/batch-rename/licenses/`. Development tools such as Vite and TypeScript are not shipped in the runtime package. Versions are recorded in package-lock.json, go.sum and the Go binary build information.

TOS and TerraMaster names identify the intended compatibility platform only. No TerraMaster artwork, proprietary icon font, wallpaper or source code is included. The application, window content and favicon use the same independently authored rename symbol.
