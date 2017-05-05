// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import "html/template"

var translations = map[string]translation{
	"en": enTranslation,
	"pl": plTranslation,
}

type translation map[string]string

func (t translation) translate(s string) string {
	if t[s] != "" {
		return t[s]
	}
	return s
}

func (t translation) htmlTranslate(s string) template.HTML {
	return template.HTML(t.translate(s))
}

var enTranslation = translation{
	"lang-code": "en",

	"login|Submit": "Submit",
	"edit|Submit":  "Submit",
}

var plTranslation = translation{
	"lang-code": "pl",

	"# No such notes":                 "# Brak takich notatek",
	"Add note":                        "Dodaj notatkę",
	"Bad request: error parsing form": "Błędne zapytanie: błąd parsowania formularza",
	"Cancel":            "Anuluj",
	"Connection error.": "Błąd połączenia.",
	"Copy":              "Kopiuj",
	"Diff":              "Porównaj",
	"Edit":              "Edytuj",
	"Error":             "Błąd",
	"Incorrect login or password.": "Niepoprawny login lub hasło.",
	"Internal server error":        "Wewnętrzny błąd serwera",
	"Login":                        "Login",
	"Logout":                       "Wyloguj",
	"Method not allowed":           "Niedozwolona metoda",
	"No differences found.":        "Nie znaleziono żadnych zmian.",
	"Page not found":               "Strona nie istnieje",
	"Password":                     "Hasło",
	"Please specify at least one topic or tag.": "Proszę podać conajmniej jeden temat lub etykietę.",
	"Please use POST.":                          "Proszę użyć POST.",
	"Preview":                                   "Podgląd",
	"Search...":                                 "Szukaj...",
	"Tags":                                      "Etykiety",
	"Topics and tags":                           "Tematy i etykiety",
	"Topics":                                    "Tematy",
	"edit|Submit":                               "Zapisz",
	"login|Submit":                              "Zaloguj się",
	"unsupported action":                        "Niewspierana akcja",
	`" and "`:                                   `" i "`,
	`Conflicting edits detected. Please join the changes and click "Submit" again when done.`:                 `Wykryto konflikt edycji. Proszę połącz zmiany i gdy zakończysz kliknij ponownie "Zapisz"`,
	`Note that the following tags/topics are new: "%s".`:                                                      `Zauważ, że następujące tematy/etykiety są nowe: "%s".`,
	`Note to login you need to have <a href="https://en.wikipedia.org/wiki/HTTP_cookie">cookies</a> enabled.`: `Aby się zalogować musisz mieć aktywne <a href="https://en.wikipedia.org/wiki/HTTP_cookie">cookie</a>.`,
	`You are adding the following tags/topics: "%s".`:                                                         `Dodajesz następujące tematy/etykiety: "%s".`,
	`You are removing the following tags/topics: "%s".`:                                                       `Usuwasz następujące tematy/etykiety: "%s".`,
}
