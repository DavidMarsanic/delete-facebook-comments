# delete-facebook-comments

**Run in terminal:** 

/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --remote-debugging-port=9222 --user-data-dir="/tmp/chrome_temp”

**How to locate the illusive confirm button element:**

Here are the key attributes and their potential uses:

aria-label="Remove": This attribute indicates that the element has an accessibility label of "Remove". This can be a useful locator if it's unique on the page.

Selenium Locator: driver.findElement(By.cssSelector("[aria-label='Remove']"))
role="button": This attribute indicates that the element is intended to be used as a button. This can be combined with the aria-label for a more specific locator.

Selenium Locator: driver.findElement(By.cssSelector("[aria-label='Remove'][role='button']"))
tabindex="0": This attribute makes the element focusable using the keyboard. It's not unique by itself, but it can be combined with other attributes for a more specific locator.

class Attributes: The element and its children have multiple class names. While these look like they might be dynamically generated (and thus might change over time), you can use a subset of them if you find any that are stable. However, be cautious, as these can change with site updates.

Text Content "Remove": The innermost span element contains the text "Remove". This can be a useful locator if it's unique on the page.

Selenium Locator (XPath): driver.findElement(By.xpath("//span[text()='Remove']"))
data-visualcompletion="ignore": This attribute might indicate that the element is ignored for visual completion checks. It's not unique by itself, but it can be combined with other attributes if needed.

Recommendations:
The aria-label and the text content "Remove" are the most stable and human-readable attributes in this element. They are likely the best candidates for creating Selenium locators.
The class names appear to be dynamically generated and might not be reliable for long-term use. However, if you're writing a short-term script or if you've observed that certain class names remain consistent, you can use them.
Always test your locators to ensure they reliably select the desired element, especially after any updates to the webpage.



Issue: 

confirm button: 
<div aria-label="Remove" class="x1i10hfl xjbqb8w x6umtig x1b1mbwd xaqea5y xav7gou x1ypdohk xe8uvvx xdj266r x11i5rnm xat24cr x1mh8g0r xexx8yu x4uap5 x18d9i69 xkhd6sd x16tdsg8 x1hl2dhg xggy1nq x1o1ewxj x3x9cwd x1e5q0jg x13rtm0m x87ps6o x1lku1pv x1a2a7pz x9f619 x3nfvp2 xdt5ytf xl56j7k x1n2onr6 xh8yej3" role="button" tabindex="0"><div class="x1n2onr6 x1ja2u2z x78zum5 x2lah0s xl56j7k x6s0dn4 xozqiw3 x1q0g3np xi112ho x17zwfj4 x585lrc x1403ito x972fbf xcfux6l x1qhh985 xm0m39n x9f619 xbxaen2 x1u72gb5 xtvsq51 x1r1pt67"><div class="x6s0dn4 x78zum5 xl56j7k x1608yet xljgi0e x1e0frkt"><div class="x9f619 x1n2onr6 x1ja2u2z x193iq5w xeuugli x6s0dn4 x78zum5 x2lah0s x1fbi1t2 xl8fo4v"><span class="x193iq5w xeuugli x13faqbe x1vvkbs xlh3980 xvmahel x1n0sxbx x1lliihq x1s928wv xhkezso x1gmr53x x1cpjm7i x1fgarty x1943h6x x4zkp8e x3x7a5m x6prxxf xvq8zen x1s688f xtk6v10" dir="auto"><span class="x1lliihq x6ikm8r x10wlt62 x1n2onr6 xlyipyv xuxw1ft">Remove</span></span></div></div><div class="x1o1ewxj x3x9cwd x1e5q0jg x13rtm0m x1ey2m1c xds687c xg01cxk x47corl x10l6tqk x17qophe x13vifvy x1ebt8du x19991ni x1dhq9h x1wpzbip" data-visualcompletion="ignore"></div></div></div>

remove button: 
<div aria-label="Remove" class="x1i10hfl xjbqb8w x6umtig x1b1mbwd xaqea5y xav7gou x1ypdohk xe8uvvx xdj266r x11i5rnm xat24cr x1mh8g0r xexx8yu x4uap5 x18d9i69 xkhd6sd x16tdsg8 x1hl2dhg xggy1nq x1o1ewxj x3x9cwd x1e5q0jg x13rtm0m x87ps6o x1lku1pv x1a2a7pz x9f619 x3nfvp2 xdt5ytf xl56j7k x1n2onr6 xh8yej3" role="button" tabindex="0"><div class="x1n2onr6 x1ja2u2z x78zum5 x2lah0s xl56j7k x6s0dn4 xozqiw3 x1q0g3np xi112ho x17zwfj4 x585lrc x1403ito x972fbf xcfux6l x1qhh985 xm0m39n x9f619 xn6708d x1ye3gou x1qhmfi1 x1r1pt67" style="transform: none;"><div class="x6s0dn4 x78zum5 xl56j7k x1608yet xljgi0e x1e0frkt"><div class="x9f619 x1n2onr6 x1ja2u2z x193iq5w xeuugli x6s0dn4 x78zum5 x2lah0s x1fbi1t2 xl8fo4v"><i data-visualcompletion="css-img" class="x1b0d499 xep6ejk" style="background-image: url(&quot;https://static.xx.fbcdn.net/rsrc.php/v3/yR/r/ZPQ1wTgIr2t.png?_nc_eui2=AeFlwoCGwv8_YYbaYj2Eh7UNdXAxVopWxJV1cDFWilbElWo5NH-bdoV9N9Cg6VxC47c&quot;); background-position: 0px -332px; background-size: auto; width: 16px; height: 16px; background-repeat: no-repeat; display: inline-block;"></i></div><div class="x9f619 x1n2onr6 x1ja2u2z x193iq5w xeuugli x6s0dn4 x78zum5 x2lah0s x1fbi1t2 xl8fo4v"><span class="x193iq5w xeuugli x13faqbe x1vvkbs xlh3980 xvmahel x1n0sxbx x1lliihq x1s928wv xhkezso x1gmr53x x1cpjm7i x1fgarty x1943h6x x4zkp8e x3x7a5m x6prxxf xvq8zen x1s688f x1dem4cn" dir="auto"><span class="x1lliihq x6ikm8r x10wlt62 x1n2onr6 xlyipyv xuxw1ft">Remove</span></span></div></div><div class="x1o1ewxj x3x9cwd x1e5q0jg x13rtm0m x1ey2m1c xds687c xg01cxk x47corl x10l6tqk x17qophe x13vifvy x1ebt8du x19991ni x1dhq9h x1wpzbip" data-visualcompletion="ignore"></div></div></div>

The way to solve the issue was to ask ChatGPT to find a way to differentiate between these two. And it found a way. 