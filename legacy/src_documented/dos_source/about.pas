{ ===========================================================================
  Unit:  about
  Role:  "About Per%Sense" modal dialog box.

  This is a pure VCL form unit (no logic). It declares the TAboutBox form
  whose visual layout lives in the companion about.dfm resource (linked via
  the *.dfm resource directive). The form simply presents static product
  identity to the user:
  program icon, product name, version, copyright, and free-form comments.

  Typically shown from a Help > About... menu command. The single OK button
  closes the dialog (the close behaviour is wired entirely in the .dfm via
  the button's ModalResult; no event handler code is needed here).

  { Go port: n/a -- static About dialog; no financial logic and no web
    equivalent (product identity is rendered directly in
    cmd/persense/static/index.html). }
  ===========================================================================}
unit about;

interface

uses Windows, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls;

type
  { TAboutBox - the About dialog form. All members below are auto-instantiated
    VCL controls owned by the form and laid out in about.dfm. }
  TAboutBox = class(TForm)
    { container panel framing the dialog content }
    Panel1: TPanel;
    { dismisses the dialog (ModalResult set in .dfm) }
    OKButton: TButton;
    { application/product icon }
    ProgramIcon: TImage;
    { product name caption (e.g. "Per%Sense") }
    ProductName: TLabel;
    { version string }
    Version: TLabel;
    { copyright notice }
    Copyright: TLabel;
    { free-form descriptive text }
    Comments: TLabel;
  private
    { Private declarations }
  public
    { Public declarations }
  end;

var
  { global singleton instance, auto-created by the VCL }
  AboutBox: TAboutBox;

implementation

{$R *.dfm}

end.

