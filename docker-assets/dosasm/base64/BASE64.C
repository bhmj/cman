#include <stdio.h>
#include <stdlib.h>

const char base64_table[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

void encode_block (unsigned char in[3], int len) {
  int i;
  unsigned char out[4];

  unsigned char i0 = in[0] >> 2;
  unsigned char i1 = ((in[0] & 0x3) << 4) | ((len > 1 ? in[1] : 0) >> 4);
  unsigned char i2 = ((in[1] & 0xF) << 2) | ((len > 2 ? in[2] : 0) >> 6);
  unsigned char i3 = in[2] & 0x3F;

  out[0] = base64_table[i0];
  out[1] = base64_table[i1];
  out[2] = len > 1 ? base64_table[i2] : '=';
  out[3] = len > 2 ? base64_table[i3] : '=';

  for (i=0; i<4; i++) {
    putchar(out[i]);
  }
}

int main (int argc, char* argv[]) {
  FILE *f;
  unsigned char in[3];
  int len;

  if (argc != 2) {
    printf("Base64 encoder\nUsage: %s <binary_file>\n", argv[0]);
    return 1;
  }

  f = fopen(argv[1], "rb");
  if (!f) {
    printf("error: cannot open file %s\n", argv[1]);
    return 1;
  }

  while (!feof(f)) {
    len = fread(in, 1, 3, f);
    if (len > 0) {
      encode_block(in, len);
    }
  }

  fclose(f);
  return 0;
}