# go-xpt
XPT dataset read/write support in Golang according to [the official SAS documentation](https://support.sas.com/content/dam/SAS/support/en/technical-papers/record-layout-of-a-sas-version-5-or-6-data-set-in-sas-transport-xport-format.pdf).

Still a work in progress.

## Reading XPT Files

Functional, but missing bells and whistles such as formats. Can do character and numerics just fine. The XPT file is read into a Dataset struct and the variable names and data are stored in Dataset.Vars. For the purposes of another project I have going on in parallel, I am really only interested in getting the XPT dataset represented as a 2D string array, and the Dataset.AsSimpleGrid() function covers this.

## Writing XPT Files

On the to-do list!