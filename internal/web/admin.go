package web

const AdminFontLink = `<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin><link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600&display=swap" rel="stylesheet">`

// AdminDashboardHTML is the admin console (/admin, and /user for the user
// role): styles in admin_css.go, markup in admin_body.go, script in admin_script.go.
const AdminDashboardHTML = `<!doctype html><html><head><meta charset="utf-8">` + ThemeHead + CSRFHead + `
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Matching Engine — Console</title>` + AdminFontLink + `
<style>` + adminCSS + adminLightCSS + `</style></head><body>
` + adminBody + `<script>` + adminJS + `</script></body></html>`
