package cmd

import (
	"github.com/spf13/cobra"
)

// araraBanner is the jp2a render of the brand mark. Hidden command: not in
// help, found by the curious -- which is the whole point of an easter egg.
const araraBanner = `
                                   ....'.  ...
                                ';cccccc  .cccc:,.
                             .;ccccc.     ccccccccc'
                           .:cccc.        cccccccccc;
                         .:ccccc    :cc.  ;cccccccccc.
                       .:cccccc             ccccccccc'
                    .,cccccccccc.         .      cccc
                     .'. ccccccc;         .c:;,. .cc.
                         cccccccc.         ;cc   .c
                         'cccccccc,
                          cccccccccc'.
                          ,ccccccccccc:'.
                          ccccccccccccccc:,.
                        .:cccccccccccccccccc:.
                       ;cccccccccccccccccccccc,
                     'ccccccccccccccccccccccccc.
                    :cccccccccccccccccccccccccc.
                  'cccccccccccccccccccccccccccc
                 ;cccccccccccccc  cccccccccccc'
               .cccccccccccccccc  ccccccccccc,
              .cccccccccccccccc'  cccccccccc'
             ;ccccccccccccccccc  'ccccccccc
            ;ccccccccccccccccc.  cccccccc;
           :ccccccccccccccccc.  cccccccc
         .cccccccccccccccccc   :cccccc.
        .ccccccccccccccccc.  .cccccc.
       'cccccccccccccccc'   ;ccccc
      ,ccccccccccccccc.   ,ccccc
     ,ccccccccccccc.   .,ccccccc,
    ;cccccccccc;     .:ccccc.  cc;
   :ccccc'       .,:cccc: .cc   ;c:
            .',:ccccccc;. .ccc. 'ccc.
          ;ccccccccccccccccccccccccccccc.
         :cccccccc,
        .cccccccc,
        cccccccc:
       ;cccccccc
       cccccccc
      :ccccccc.
     .ccccccc'
     ccccccc,
    .cccccc;
    cccccc:
   ;ccccc:
   cccccc
  :cccc:
  cccc:
 :ccc;
 ccc'
,cc
:

  AraraHQ -- a operacao de WhatsApp para empresas.
  Gostou do que viu por baixo do capo? oi@ararahq.com
`

var parrotCmd = &cobra.Command{
	Use:    "parrot",
	Short:  "Prints the arara",
	Hidden: true,
	RunE:   runParrot,
}

func init() {
	rootCmd.AddCommand(parrotCmd)
}

func runParrot(command *cobra.Command, arguments []string) error {
	command.Print(araraBanner)
	return nil
}
