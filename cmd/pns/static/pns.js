// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

new Awesomplete(Awesomplete.$('input[data-multiple]'), {
	autoFirst: true,
	minChars: 1,

	filter: function(text, input) {
		return Awesomplete.FILTER_CONTAINS(text, input.match(/[^- ][^ ]*$|$/)[0]);
	},

	replace: function(text) {
		var before = this.input.value.match(/^.+\s+-?|-?/)[0];
		this.input.value = before + text + " ";
	}
});
