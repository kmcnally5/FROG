// Simple multi-column form
fn main() {
	// Form state
	name = ""
	email = ""
	category = ""
	description = ""
	submitted = false

	// Categories for dropdown
	categories = ["Electronics", "Software", "Hardware", "Other"]

	window(800, 500, "kLex Form Example", fn(frameCount) {
		background(0.15, 0.15, 0.15)

		uiBegin()

		// ── Left Column (Input Fields)
		label("Contact Information", 50, 30, 0.8)

		name = textInput("Name:", name, 150, 60, 300, 35, 0.6)

		email = textInput("Email:", email, 150, 110, 300, 35, 0.6)

		category = list("Category:", categories, 50, 160, 300, 120, 0.6)

		// ── Right Column (Description & Buttons)
		label("Additional Info", 450, 30, 0.8)

		description = textInput("Notes:", description, 450, 60, 300, 200, 0.6)

		// ── Action Buttons
		if (button("Submit", 450, 280, 140, 40, 0.75)) {
			submitted = true
		}

		if (button("Clear", 610, 280, 140, 40, 0.75)) {
			name = ""
			email = ""
			description = ""
		}

		// ── Display Results
		if (submitted) {
			label("Submitted!", 50, 360, 0.9)
			label("Name: " + name, 50, 390, 0.7)
			label("Email: " + email, 50, 410, 0.7)
			label("Category: " + category, 50, 430, 0.7)
		}

		uiEnd()
	})
}

main()
