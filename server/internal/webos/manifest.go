package webos

// manifest is sent when registering with the TV. Older webOS versions (e.g. 2020 models) only grant
// READ_POWER_STATE to a manifest that carries LG's signed block, so the publicly known signed block of
// LG's test remote app is included unchanged (as used by PyWebOSTV, MIT License, and aiowebostv).
// The signature covers only "signed"; the permissions requested by this app are listed separately
// and limited to what it needs.
var manifest = map[string]any{
	"manifestVersion": 1,
	"appVersion":      "1.1",
	"permissions":     []string{"READ_POWER_STATE"},
	"signed": map[string]any{
		"appId":   "com.lge.test",
		"created": "20140509",
		"localizedAppNames": map[string]string{
			"":       "LG Remote App",
			"ko-KR":  "리모컨 앱",
			"zxx-XX": "ЛГ Rэмotэ AПП",
		},
		"localizedVendorNames": map[string]string{"": "LG Electronics"},
		"permissions": []string{
			"TEST_SECURE",
			"CONTROL_INPUT_TEXT",
			"CONTROL_MOUSE_AND_KEYBOARD",
			"READ_INSTALLED_APPS",
			"READ_LGE_SDX",
			"READ_NOTIFICATIONS",
			"SEARCH",
			"WRITE_SETTINGS",
			"WRITE_NOTIFICATION_ALERT",
			"CONTROL_POWER",
			"READ_CURRENT_CHANNEL",
			"READ_RUNNING_APPS",
			"READ_UPDATE_INFO",
			"UPDATE_FROM_REMOTE_APP",
			"READ_LGE_TV_INPUT_EVENTS",
			"READ_TV_CURRENT_TIME",
		},
		"serial":   "2f930e2d2cfe083771f68e4fe7bb07",
		"vendorId": "com.lge",
	},
	"signatures": []map[string]any{{
		"signatureVersion": 1,
		"signature": "eyJhbGdvcml0aG0iOiJSU0EtU0hBMjU2Iiwia2V5SWQiOiJ0ZXN0LXNpZ25pbm" +
			"ctY2VydCIsInNpZ25hdHVyZVZlcnNpb24iOjF9.hrVRgjCwXVvE2OOSpDZ58hR" +
			"+59aFNwYDyjQgKk3auukd7pcegmE2CzPCa0bJ0ZsRAcKkCTJrWo5iDzNhMBWRy" +
			"aMOv5zWSrthlf7G128qvIlpMT0YNY+n/FaOHE73uLrS/g7swl3/qH/BGFG2Hu4" +
			"RlL48eb3lLKqTt2xKHdCs6Cd4RMfJPYnzgvI4BNrFUKsjkcu+WD4OO2A27Pq1n" +
			"50cMchmcaXadJhGrOqH5YmHdOCj5NSHzJYrsW0HPlpuAx/ECMeIZYDh6RMqaFM" +
			"2DXzdKX9NmmyqzJ3o/0lkk/N97gfVRLW5hA29yeAwaCViZNCP8iC9aO0q9fQoj" +
			"oa7NQnAtw==",
	}},
}
