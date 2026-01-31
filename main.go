/*
	go-xpt: an open-source, Go solution to reading/writing XPT (SAS Transport) files.
    Copyright (C) 2026  Jan van der Linde

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package goxpt

import "fmt"

const disclaimer string = "go-xpt  Copyright (C) 2026 Jan van der Linde\nThis program comes with ABSOLUTELY NO WARRANTY."

func main() {
	fmt.Println(disclaimer)
}
