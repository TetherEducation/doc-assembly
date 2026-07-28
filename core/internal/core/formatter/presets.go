package formatter

import "github.com/TetherEducation/doc-assembly/core/internal/core/entity"

// DateFormats provides common date format options.
var DateFormats = &entity.FormatConfig{
	Default: "DD/MM/YYYY",
	Options: []string{
		"DD/MM/YYYY",   // 31/12/2024
		"MM/DD/YYYY",   // 12/31/2024
		"YYYY-MM-DD",   // 2024-12-31 (ISO)
		"D MMMM YYYY",  // 31 December 2024
		"MMMM D, YYYY", // December 31, 2024
		"DD MMM YYYY",  // 31 Dec 2024
	},
}

// TimeFormats provides common time format options.
var TimeFormats = &entity.FormatConfig{
	Default: "HH:mm",
	Options: []string{
		"HH:mm",      // 14:30 (24h)
		"HH:mm:ss",   // 14:30:45
		"hh:mm a",    // 02:30 PM (12h)
		"hh:mm:ss a", // 02:30:45 PM
	},
}

// DateTimeFormats provides common datetime format options.
var DateTimeFormats = &entity.FormatConfig{
	Default: "DD/MM/YYYY HH:mm",
	Options: []string{
		"DD/MM/YYYY HH:mm",     // 31/12/2024 14:30
		"YYYY-MM-DD HH:mm:ss",  // 2024-12-31 14:30:45 (ISO)
		"D MMMM YYYY, HH:mm",   // 31 December 2024, 14:30
		"MMMM D, YYYY hh:mm a", // December 31, 2024 02:30 PM
		"DD/MM/YYYY hh:mm a",   // 31/12/2024 02:30 PM
	},
}

// NumberFormats provides common number format options.
var NumberFormats = &entity.FormatConfig{
	Default: "#,##0.00",
	Options: []string{
		"#,##0.00",  // 1,234.56
		"#,##0",     // 1,235 (no decimals)
		"#,##0.000", // 1,234.560 (3 decimals)
		"0.00",      // 1234.56 (no thousands separator)
	},
}

// CurrencyFormats provides common currency format options.
var CurrencyFormats = &entity.FormatConfig{
	Default: "$#,##0.00",
	Options: []string{
		"$#,##0.00",    // $1,234.56
		"€#,##0.00",    // €1,234.56
		"#,##0.00 USD", // 1,234.56 USD
		"#,##0.00 €",   // 1,234.56 €
	},
}

// PercentageFormats provides common percentage format options.
var PercentageFormats = &entity.FormatConfig{
	Default: "#,##0.00%",
	Options: []string{
		"#,##0.00%", // 12.34%
		"#,##0%",    // 12%
		"#,##0.0%",  // 12.3%
	},
}

// PhoneFormats provides common phone format options.
var PhoneFormats = &entity.FormatConfig{
	Default: "+## # #### ####",
	Options: []string{
		"+## # #### ####", // +56 9 1234 5678
		"(###) ###-####",  // (123) 456-7890
		"### ### ####",    // 123 456 7890
		"+##-#-####-####", // +56-9-1234-5678
	},
}

// RUTFormats provides Chilean RUT format options.
var RUTFormats = &entity.FormatConfig{
	Default: "##.###.###-#",
	Options: []string{
		"##.###.###-#", // 12.345.678-9
		"########-#",   // 12345678-9
	},
}

// BoolFormats provides boolean display format options.
var BoolFormats = &entity.FormatConfig{
	Default: "Yes/No",
	Options: []string{
		"Yes/No",
		"True/False",
		"Sí/No",
	},
}
