package gofastrtf

import (
	"testing"

	"github.com/AOShei/go-fast-core"
)

// wordStyle is a document in the shape Word writes: font table with
// panose groups, a style sheet with latent styles, rsid tables, theme data,
// runs wrapped in groups with associated fonts, and paragraphs closed by
// \par inside their last run's group.
const wordStyle = `{\rtf1\adeflang1025\ansi\ansicpg1252\uc1\adeff31507\deff0\stshfdbch31505\deflang1033\deflangfe1033\themelang1033` +
	`{\fonttbl{\f0\fbidi \froman\fcharset0\fprq2{\*\panose 02020603050405020304}Times New Roman;}` +
	`{\f37\fbidi \fswiss\fcharset0\fprq2{\*\panose 020f0502020204030204}Calibri;}` +
	`{\f39\fbidi \froman\fcharset238\fprq2 Times New Roman CE;}}` +
	`{\colortbl;\red0\green0\blue0;\red0\green0\blue255;\red255\green255\blue255;}` +
	`{\*\defchp \f31506\fs22 }{\*\defpap \ql \li0\ri0\sa160\sl259\slmult1}` +
	`{\stylesheet{\ql \li0\ri0\sa160\sl259\slmult1\widctlpar\wrapdefault\aspalpha\aspnum\faauto\adjustright\rin0\lin0\itap0 \rtlch\fcs1 \af31507\afs22\alang1025 \ltrch\fcs0 \f31506\fs22\lang1033\langfe1033\cgrid\langnp1033\langfenp1033 \snext0 \sqformat \spriority0 Normal;}` +
	`{\s1\ql \li0\ri0\sb240\keepn\widctlpar\wrapdefault\aspalpha\aspnum\faauto\outlinelevel0\adjustright\rin0\lin0\itap0 \rtlch\fcs1 \af31503\afs32 \ltrch\fcs0 \fs32\cf19 \sbasedon0 \snext0 \slink15 \sqformat \spriority9 \styrsid1 heading 1;}` +
	`{\*\cs10 \additive \ssemihidden \sunhideused \spriority1 Default Paragraph Font;}` +
	`{\*\ts11\tsrowd\trftsWidthB3\trpaddl108\trpaddr108\trpaddfl3\trpaddft3\trpaddfb3\trpaddfr3\tblind0\tblindtype3\tsvertalt\tsbrdrt\tsbrdrl\tsbrdrb\tsbrdrr\tsbrdrdgl\tsbrdrdgr\tsbrdrh\tsbrdrv \ql \li0\ri0\sa160\sl259\slmult1\widctlpar\wrapdefault\aspalpha\aspnum\faauto\adjustright\rin0\lin0\itap0 \rtlch\fcs1 \af31507\afs22\alang1025 \ltrch\fcs0 \f31506\fs22\lang1033\langfe1033\cgrid\langnp1033\langfenp1033 \snext11 \ssemihidden \sunhideused Normal Table;}}` +
	`{\*\rsidtbl \rsid1\rsid2}{\mmathPr\mmathFont34\mbrkBin0}` +
	`{\info{\title Quarterly}{\author Jane Doe}{\operator Jane Doe}{\creatim\yr2026\mo3\dy4\hr9\min5}{\version2}{\edmins1}{\nofpages1}{\nofwords5}{\nofchars30}{\*\company Example Ltd}{\vern57}}` +
	`{\*\xmlnstbl {\xmlns1 http://schemas.microsoft.com/office/word/2003/wordml}}` +
	`\paperw12240\paperh15840\margl1440\margr1440\margt1440\margb1440\gutter0\ltrsect` +
	`\widowctrl\ftnbj\aenddoc\trackmoves0\trackformatting1\donotembedsysfont1\relyonvml0\donotembedlingdata0\grfdocevents0\validatexml1\showplaceholdtext0\ignoremixedcontent0\saveinvalidxml0\showxmlerrors1\noxlattoyen` +
	`{\*\wgrffmtfilter 2450}\nofeaturethrottle1\ilfomacatclnup0\ltrpar \sectd \ltrsect\linex0\endnhere\sectlinegrid360\sectdefaultcl\sftnbj ` +
	`{\*\pnseclvl1\pnucrm\pnstart1\pnindent720\pnhang {\pntxta .}}` +
	`\pard\plain \ltrpar\s1\ql \li0\ri0\sb240\keepn\widctlpar\wrapdefault\aspalpha\aspnum\faauto\outlinelevel0\adjustright\rin0\lin0\itap0\pararsid1 \rtlch\fcs1 \af31503\afs32\alang1025 \ltrch\fcs0 \fs32\cf19\lang1033\langfe1033\loch\af31502\hich\af31502\dbch\af31501\cgrid\langnp1033\langfenp1033 {\rtlch\fcs1 \af31503 \ltrch\fcs0 \insrsid1 \hich\af31502\dbch\af31501\loch\f31502 Quarterly report}{\rtlch\fcs1 \af31503 \ltrch\fcs0 \insrsid1 \par }` +
	`\pard\plain \ltrpar\ql \li0\ri0\sa160\sl259\slmult1\widctlpar\wrapdefault\aspalpha\aspnum\faauto\adjustright\rin0\lin0\itap0 \rtlch\fcs1 \af31507\afs22\alang1025 \ltrch\fcs0 \f31506\fs22\lang1033\langfe1033\cgrid\langnp1033\langfenp1033 {\rtlch\fcs1 \af31507 \ltrch\fcs0 \insrsid1 Revenue grew in }{\rtlch\fcs1 \af31507 \ltrch\fcs0 \b\insrsid1 every}{\rtlch\fcs1 \af31507 \ltrch\fcs0 \insrsid1  region. }{\field{\*\fldinst {\rtlch\fcs1 \af31507 \ltrch\fcs0 \insrsid1 HYPERLINK "https://example.com/" }{\rtlch\fcs1 \af31507 \ltrch\fcs0 \insrsid1 {\*\datafield 00d0c9ea79f9bace118c8200aa004ba90b02000000}}}{\fldrslt {\rtlch\fcs1 \af31507 \ltrch\fcs0 \cs15\ul\cf2\insrsid1 Details}}}\sectd \ltrsect\linex0\endnhere\sectlinegrid360\sectdefaultcl\sftnbj {\rtlch\fcs1 \af31507 \ltrch\fcs0 \insrsid1 \par }` +
	`{\*\themedata 504b030414000600080000002100e9de0fbfff0000001c020000130000005b436f6e74656e745f54797065735d2e786d6cac91cb4ec3301045f748fc83e52d4a}` +
	`{\*\colorschememapping 3c3f786d6c2076657273696f6e3d22312e302220656e636f64696e673d225554462d3822207374616e64616c6f6e653d22796573223f3e}` +
	`{\*\latentstyles\lsdstimax376\lsdlockeddef0\lsdsemihiddendef0{\lsdlockedexcept \lsdqformat1 \lsdpriority0 \lsdlocked0 Normal;}}` +
	`{\*\datastore 01050000020000001800000057006f00720064002e0050006900630074007500720065002e0038000000000000000000000000}}`

func TestWordStyle(t *testing.T) {
	doc := extract(t, wordStyle, nil)
	bs := blocks(doc)
	if len(bs) != 3 {
		t.Fatalf("got %s", dump(doc))
	}
	if bs[0].Kind != gofastcore.KindHeading || bs[0].Level != 1 || bs[0].Text != "Quarterly report" {
		t.Errorf("heading %+v", bs[0])
	}
	if bs[1].Text != "Revenue grew in every region. Details" {
		t.Errorf("paragraph %q", bs[1].Text)
	}
	if bs[2].Kind != gofastcore.KindNote || bs[2].Text != `HYPERLINK "https://example.com/"` {
		t.Errorf("field %+v", bs[2])
	}
	m := doc.Metadata
	if m.Title != "Quarterly" || m.Authors[0] != "Jane Doe" || m.Extra["company"] != "Example Ltd" || m.Created.Year() != 2026 || m.Language != "en-US" {
		t.Errorf("metadata %+v", m)
	}
	for _, w := range doc.Parts[0].Warnings {
		t.Logf("warning %+v", w)
	}
}
