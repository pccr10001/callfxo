package li.power.app.callfxo.android.data

import android.provider.ContactsContract
import android.content.Context

data class PhoneContact(
  val id: String,
  val name: String,
  val number: String,
  val source: ContactSource,
)

enum class ContactSource { SERVER, DEVICE }

class DeviceContactsReader {
  fun readAll(context: Context): List<PhoneContact> {
    val out = mutableListOf<PhoneContact>()
    val uri = ContactsContract.CommonDataKinds.Phone.CONTENT_URI
    val projection = arrayOf(
      ContactsContract.CommonDataKinds.Phone._ID,
      ContactsContract.CommonDataKinds.Phone.DISPLAY_NAME,
      ContactsContract.CommonDataKinds.Phone.NUMBER,
    )
    context.contentResolver.query(uri, projection, null, null, null)?.use { cur ->
      val idIdx = cur.getColumnIndexOrThrow(ContactsContract.CommonDataKinds.Phone._ID)
      val nameIdx = cur.getColumnIndexOrThrow(ContactsContract.CommonDataKinds.Phone.DISPLAY_NAME)
      val numIdx = cur.getColumnIndexOrThrow(ContactsContract.CommonDataKinds.Phone.NUMBER)
      while (cur.moveToNext()) {
        val id = cur.getString(idIdx).orEmpty()
        val name = cur.getString(nameIdx).orEmpty()
        val number = cur.getString(numIdx).orEmpty()
        if (number.isNotBlank()) {
          out += PhoneContact(id = "d-$id", name = name, number = number, source = ContactSource.DEVICE)
        }
      }
    }
    return out
  }
}
